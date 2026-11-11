package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quitordinary/terraform-provider-netcup/internal/scptest"
)

func TestLoginPrintsOnlyTheRefreshTokenToStdout(t *testing.T) {
	_, srv := scptest.New(t)
	t.Setenv(envBaseURL, srv.URL)

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"login"}, &stdout, &stderr); err != nil {
		t.Fatalf("login: %v", err)
	}
	if got := stdout.String(); got != "refresh-0\n" {
		t.Errorf("stdout = %q, want exactly the refresh token", got)
	}
	if strings.Contains(stderr.String(), "refresh-0") || strings.Contains(stderr.String(), "access-0") {
		t.Errorf("stderr leaks a token: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "ABCD-EFGH") {
		t.Errorf("stderr does not show the user code: %q", stderr.String())
	}
}

func TestTokenCheckNeverPrintsTokens(t *testing.T) {
	realm, srv := scptest.New(t)
	realm.Rotate = true
	t.Setenv(envBaseURL, srv.URL)
	t.Setenv(envRefreshToken, "refresh-0")

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"token", "check", "--reuse"}, &stdout, &stderr); err != nil {
		t.Fatalf("token check: %v", err)
	}
	out := stdout.String()
	for _, secret := range []string{"refresh-0", "refresh-1", "access-0"} {
		if strings.Contains(out, secret) {
			t.Errorf("output leaks %q:\n%s", secret, out)
		}
	}
	for _, want := range []string{"new refresh token:       true", "original token reusable: false"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestTokenRenewPrintsOnlyTheNewestToken(t *testing.T) {
	realm, srv := scptest.New(t)
	realm.Rotate = true
	t.Setenv(envBaseURL, srv.URL)
	t.Setenv(envRefreshToken, "refresh-0")

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"token", "renew"}, &stdout, &stderr); err != nil {
		t.Fatalf("token renew: %v", err)
	}
	if got := stdout.String(); got != "refresh-1\n" {
		t.Errorf("stdout = %q, want exactly the newest token", got)
	}
	if strings.Contains(stderr.String(), "refresh-") {
		t.Errorf("stderr leaks a token: %q", stderr.String())
	}
}

func TestTokenRenewKeepsATokenTheRealmDidNotReplace(t *testing.T) {
	_, srv := scptest.New(t) // no rotation: the realm answers with the same token
	t.Setenv(envBaseURL, srv.URL)
	t.Setenv(envRefreshToken, "refresh-0")

	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"token", "renew"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("token renew: %v", err)
	}
	if got := stdout.String(); got != "refresh-0\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestSpecFetchAndPaths(t *testing.T) {
	_, srv := scptest.New(t)
	t.Setenv(envBaseURL, srv.URL)
	t.Setenv(envRefreshToken, "refresh-0")
	file := filepath.Join(t.TempDir(), "openapi.json")

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"spec", "fetch", "-o", file}, &stdout, &stderr); err != nil {
		t.Fatalf("spec fetch: %v", err)
	}
	if !strings.Contains(stderr.String(), "2 paths") {
		t.Errorf("summary: %q", stderr.String())
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("spec file mode: %v %v", info, err)
	}

	stdout.Reset()
	if err := run(context.Background(), []string{"spec", "paths", file}, &stdout, &stderr); err != nil {
		t.Fatalf("spec paths: %v", err)
	}
	want := "GET    /api/v1/servers\nGET    /api/v1/servers/{serverId}\nPATCH  /api/v1/servers/{serverId}\n"
	if stdout.String() != want {
		t.Errorf("paths =\n%s\nwant\n%s", stdout.String(), want)
	}
}

func TestMissingRefreshTokenExplainsLogin(t *testing.T) {
	t.Setenv(envRefreshToken, "")
	err := run(context.Background(), []string{"token", "check"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "netcupctl login") {
		t.Fatalf("want a hint to log in, got %v", err)
	}
}
