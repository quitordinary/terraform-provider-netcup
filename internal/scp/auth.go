// Package scp is the client for netcup's Server Control Panel REST API.
//
// Sign-in is Keycloak OIDC with the public client "scp": a device-code login
// yields an offline refresh token, and every run exchanges that refresh token for
// short-lived access tokens held only in memory. No token is ever logged.
package scp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// DefaultBaseURL is the SCP origin; the realm and the API live below it.
const DefaultBaseURL = "https://www.servercontrolpanel.de"

const (
	clientID  = "scp"
	realmPath = "/realms/scp/protocol/openid-connect"
	apiPath   = "/scp-core/api/v1"
)

// Scopes requested at login. offline_access makes the refresh token survive the
// browser session; it still lapses after a period without use.
var Scopes = []string{"offline_access", "openid"}

// OAuthConfig returns the OAuth2 configuration for the SCP realm under baseURL.
func OAuthConfig(baseURL string) *oauth2.Config {
	base := strings.TrimRight(baseURL, "/") + realmPath
	return &oauth2.Config{
		ClientID: clientID,
		Scopes:   Scopes,
		Endpoint: oauth2.Endpoint{
			DeviceAuthURL: base + "/auth/device",
			TokenURL:      base + "/token",
			// A public client: client_id travels in the form body, no secret.
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

// Prompt tells the operator where to approve a device login. It receives no token.
type Prompt func(verificationURI, verificationURIComplete, userCode string, expiry time.Time)

// Login runs the device-code flow: it asks the realm for a code, hands the
// verification details to prompt, and polls until the operator approves in a
// browser. The returned token carries the offline refresh token.
func Login(ctx context.Context, baseURL string, httpClient *http.Client, prompt Prompt) (*oauth2.Token, error) {
	conf := OAuthConfig(baseURL)
	ctx = withHTTPClient(ctx, httpClient)

	device, err := conf.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("request a device code: %w", err)
	}
	prompt(device.VerificationURI, device.VerificationURIComplete, device.UserCode, device.Expiry)

	token, err := conf.DeviceAccessToken(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("wait for the device login to be approved: %w", err)
	}
	if token.RefreshToken == "" {
		return nil, errors.New("the realm returned no refresh token; offline_access was not granted")
	}
	return token, nil
}

// Refresh exchanges a refresh token once and returns the full token response,
// including a rotated refresh token if the realm issues one.
func Refresh(ctx context.Context, baseURL string, httpClient *http.Client, refreshToken string) (*oauth2.Token, error) {
	if refreshToken == "" {
		return nil, errors.New("no refresh token")
	}
	conf := OAuthConfig(baseURL)
	ctx = withHTTPClient(ctx, httpClient)
	// An expired token with only a refresh token forces exactly one exchange.
	token, err := conf.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		return nil, fmt.Errorf("exchange the refresh token: %w", err)
	}
	return token, nil
}

// TokenSource returns a concurrency-safe source of access tokens that renews
// shortly before expiry and keeps a rotated refresh token in memory.
func TokenSource(ctx context.Context, baseURL string, httpClient *http.Client, refreshToken string) oauth2.TokenSource {
	conf := OAuthConfig(baseURL)
	return conf.TokenSource(withHTTPClient(ctx, httpClient), &oauth2.Token{RefreshToken: refreshToken})
}

func withHTTPClient(ctx context.Context, httpClient *http.Client) context.Context {
	if httpClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, httpClient)
}
