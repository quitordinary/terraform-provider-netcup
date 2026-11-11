package scp_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/quitordinary/terraform-provider-netcup/internal/scp"
	"github.com/quitordinary/terraform-provider-netcup/internal/scptest"
)

func TestLoginPollsUntilApproved(t *testing.T) {
	realm, srv := scptest.New(t)
	realm.PendingPolls = 1

	var shownCode string
	token, err := scp.Login(context.Background(), srv.URL, srv.Client(), func(_, _, code string, _ time.Time) {
		shownCode = code
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if shownCode != "ABCD-EFGH" {
		t.Errorf("prompt got code %q", shownCode)
	}
	if token.RefreshToken != "refresh-0" {
		t.Errorf("refresh token %q", token.RefreshToken)
	}
}

func TestLoginRejectsAnswerWithoutRefreshToken(t *testing.T) {
	realm, srv := scptest.New(t)
	realm.RefreshToken = ""

	_, err := scp.Login(context.Background(), srv.URL, srv.Client(), func(string, string, string, time.Time) {})
	if err == nil || !strings.Contains(err.Error(), "offline_access") {
		t.Fatalf("want an offline_access error, got %v", err)
	}
}

func TestRefreshReportsRotation(t *testing.T) {
	realm, srv := scptest.New(t)
	realm.Rotate = true

	token, err := scp.Refresh(context.Background(), srv.URL, srv.Client(), "refresh-0")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if token.RefreshToken != "refresh-1" {
		t.Errorf("want the rotated token, got %q", token.RefreshToken)
	}
	if _, err := scp.Refresh(context.Background(), srv.URL, srv.Client(), "refresh-0"); err == nil {
		t.Error("the rotated-away token must be refused")
	}
}

func TestRefreshWithoutTokenFails(t *testing.T) {
	if _, err := scp.Refresh(context.Background(), "http://unused.invalid", nil, ""); err == nil {
		t.Fatal("want an error for an empty refresh token")
	}
}

func TestClientSendsBearerAndHidesItInErrors(t *testing.T) {
	realm, srv := scptest.New(t)
	ctx := context.Background()

	tokens := scp.TokenSource(ctx, srv.URL, srv.Client(), "refresh-0")
	client := scp.NewClient(ctx, srv.URL, srv.Client(), tokens, "test")
	if _, err := client.GetRaw(ctx, "/openapi"); err != nil {
		t.Fatalf("GetRaw: %v", err)
	}

	realm.SetAccessToken("access-changed") // the client's cached token is now refused
	_, err := client.GetRaw(ctx, "/openapi")
	var apiErr *scp.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("want an APIError 401, got %v", err)
	}
	for _, secret := range []string{"access-0", "access-changed", "refresh-0"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error leaks %q: %v", secret, err)
		}
	}
}
