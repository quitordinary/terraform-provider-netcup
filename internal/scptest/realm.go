// Package scptest is a fake of netcup's SCP for tests: a minimal Keycloak (device
// authorization, the device and refresh grants) and API routes behind a bearer
// token. Its paths are written out rather than imported from package scp, so the
// tests pin the real endpoint contract.
package scptest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const (
	realmPath = "/realms/scp/protocol/openid-connect"
	apiPath   = "/scp-core/api/v1"
	clientID  = "scp"
	scope     = "offline_access openid"
)

// Realm is the fake. Set its fields before the first request, or between
// requests from the same goroutine.
type Realm struct {
	t *testing.T

	mu sync.Mutex
	// PendingPolls is how many device polls are answered authorization_pending.
	PendingPolls int
	// Rotate issues a new refresh token on every refresh.
	Rotate bool
	// RefreshToken is the currently valid refresh token; empty means the realm
	// grants no offline access.
	RefreshToken string
	// AccessToken is the bearer token the API routes accept.
	AccessToken string
	issued      int
}

// New starts the fake and returns it with its server.
func New(t *testing.T) (*Realm, *httptest.Server) {
	t.Helper()
	r := &Realm{t: t, RefreshToken: "refresh-0", AccessToken: "access-0"}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+realmPath+"/auth/device", r.device)
	mux.HandleFunc("POST "+realmPath+"/token", r.token)
	mux.HandleFunc("GET "+apiPath+"/openapi", r.openapi)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return r, srv
}

// SetAccessToken changes the accepted bearer token, invalidating cached ones.
func (r *Realm) SetAccessToken(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.AccessToken = token
}

func (r *Realm) form(req *http.Request) map[string]string {
	if err := req.ParseForm(); err != nil {
		r.t.Errorf("parse form: %v", err)
	}
	if _, _, ok := req.BasicAuth(); ok {
		r.t.Errorf("a public client must not send basic auth")
	}
	form := map[string]string{}
	for k := range req.PostForm {
		form[k] = req.PostForm.Get(k)
	}
	return form
}

func (r *Realm) device(w http.ResponseWriter, req *http.Request) {
	form := r.form(req)
	if form["client_id"] != clientID || form["scope"] != scope {
		http.Error(w, "bad client or scope", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               "dev-code",
		"user_code":                 "ABCD-EFGH",
		"verification_uri":          "https://example.invalid/device",
		"verification_uri_complete": "https://example.invalid/device?user_code=ABCD-EFGH",
		"expires_in":                600,
		"interval":                  1,
	})
}

func (r *Realm) token(w http.ResponseWriter, req *http.Request) {
	form := r.form(req)
	r.mu.Lock()
	defer r.mu.Unlock()
	if form["client_id"] != clientID {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	switch form["grant_type"] {
	case "urn:ietf:params:oauth:grant-type:device_code":
		if form["device_code"] != "dev-code" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		if r.PendingPolls > 0 {
			r.PendingPolls--
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authorization_pending"})
			return
		}
	case "refresh_token":
		if form["refresh_token"] == "" || form["refresh_token"] != r.RefreshToken {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		if r.Rotate {
			r.issued++
			r.RefreshToken = fmt.Sprintf("refresh-%d", r.issued)
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":       r.AccessToken,
		"token_type":         "Bearer",
		"expires_in":         300,
		"refresh_token":      r.RefreshToken,
		"refresh_expires_in": 0,
	})
}

func (r *Realm) openapi(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	want := "Bearer " + r.AccessToken
	r.mu.Unlock()
	if req.Header.Get("Authorization") != want {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Authorization header missing."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.0.1",
		"info":    map[string]string{"version": "2026.1111.000000"},
		"paths": map[string]any{
			"/api/v1/servers":            map[string]any{"get": map[string]any{}},
			"/api/v1/servers/{serverId}": map[string]any{"get": map[string]any{}, "patch": map[string]any{}},
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
