// Command netcupctl does what the provider does not: the device-code login and
// one-off operations against netcup's Server Control Panel (ADR-0066).
//
// Tokens never reach a terminal or a log. `login` prints the refresh token only
// to a pipe or a file; every other command reads it from NETCUP_SCP_REFRESH_TOKEN.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/quitordinary/terraform-provider-netcup/internal/scp"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

const (
	envRefreshToken = "NETCUP_SCP_REFRESH_TOKEN" //nolint:gosec // G101: an environment variable's name, not a credential
	envBaseURL      = "NETCUP_SCP_BASE_URL"
)

const usage = `netcupctl: netcup SCP login and operations (ADR-0066)

Usage:
  netcupctl login                     device-code login; prints the refresh token to stdout (never to a terminal)
  netcupctl token check [--reuse]     exchange the refresh token and report lifetimes and rotation, never the token
  netcupctl spec fetch [-o FILE]      download the OpenAPI document (default openapi.json)
  netcupctl spec paths FILE           list "METHOD path" for every operation in an OpenAPI document
  netcupctl version

Environment:
  NETCUP_SCP_REFRESH_TOKEN   the offline refresh token (every command but login and spec paths)
  NETCUP_SCP_BASE_URL        override https://www.servercontrolpanel.de (tests only)
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "netcupctl:", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return errors.New("no command")
	}
	switch strings.Join(firstN(args, 2), " ") {
	case "token check":
		return tokenCheck(ctx, args[2:], stdout)
	case "spec fetch":
		return specFetch(ctx, args[2:], stderr)
	case "spec paths":
		return specPaths(args[2:], stdout)
	}
	switch args[0] {
	case "login":
		return login(ctx, args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, versionString())
		return nil
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	}
	fmt.Fprint(stderr, usage)
	return fmt.Errorf("unknown command %q", strings.Join(args, " "))
}

func login(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if isTerminal(stdout) {
		return errors.New("stdout is a terminal; pipe the token somewhere instead, " +
			`e.g. export NETCUP_SCP_REFRESH_TOKEN="$(netcupctl login)"`)
	}

	token, err := scp.Login(ctx, baseURL(), httpClient(), func(uri, complete, code string, expiry time.Time) {
		fmt.Fprintf(stderr, "Open %s\n", complete)
		fmt.Fprintf(stderr, "and check that it shows the code %s (otherwise enter it at %s).\n", code, uri)
		if !expiry.IsZero() {
			fmt.Fprintf(stderr, "The code expires at %s. Waiting for approval...\n", expiry.Local().Format(time.TimeOnly))
		}
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(stderr, "Approved. Refresh token written to stdout.")
	_, err = fmt.Fprintln(stdout, token.RefreshToken)
	return err
}

func tokenCheck(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("token check", flag.ContinueOnError)
	reuse := fs.Bool("reuse", false, "after the exchange, try the original refresh token a second time "+
		"(if the realm detects reuse, this revokes the session and you log in again)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	original, err := refreshTokenFromEnv()
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "refresh token presented:")
	printClaims(stdout, original)

	first, err := scp.Refresh(ctx, baseURL(), httpClient(), original)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "access token lifetime:   %s\n", time.Until(first.Expiry).Round(time.Second))
	fmt.Fprintf(stdout, "refresh_expires_in:      %v (Keycloak reports 0 when the session has no maximum)\n",
		first.Extra("refresh_expires_in"))
	rotated := first.RefreshToken != "" && first.RefreshToken != original
	fmt.Fprintf(stdout, "refresh token rotated:   %t\n", rotated)
	if rotated {
		fmt.Fprintln(stdout, "new refresh token:")
		printClaims(stdout, first.RefreshToken)
	}

	if !*reuse {
		return nil
	}
	_, err = scp.Refresh(ctx, baseURL(), httpClient(), original)
	fmt.Fprintf(stdout, "original token reusable: %t\n", err == nil)
	if err != nil {
		fmt.Fprintf(stdout, "  realm answered: %v\n", err)
	}
	return nil
}

func specFetch(ctx context.Context, args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("spec fetch", flag.ContinueOnError)
	out := fs.String("o", "openapi.json", "file to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	refresh, err := refreshTokenFromEnv()
	if err != nil {
		return err
	}
	tokens := scp.TokenSource(ctx, baseURL(), httpClient(), refresh)
	client := scp.NewClient(ctx, baseURL(), httpClient(), tokens, "netcupctl/"+version)

	body, err := client.GetRaw(ctx, "/openapi")
	if err != nil {
		return err
	}
	var doc struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Version string `json:"version"`
		} `json:"info"`
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("the answer is not an OpenAPI document: %w", err)
	}
	if err := os.WriteFile(*out, body, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Wrote %s: OpenAPI %s, SCP %s, %d paths, %d bytes.\n",
		*out, doc.OpenAPI, doc.Info.Version, len(doc.Paths), len(body))
	return nil
}

func specPaths(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: netcupctl spec paths FILE")
	}
	body, err := os.ReadFile(args[0]) //nolint:gosec // G703: the operator names the file to read
	if err != nil {
		return err
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	order := map[string]int{"get": 0, "post": 1, "put": 2, "patch": 3, "delete": 4}
	type operation struct{ path, method string }
	var ops []operation
	for path, item := range doc.Paths {
		for method := range item {
			if _, ok := order[method]; ok {
				ops = append(ops, operation{path, method})
			}
		}
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].path != ops[j].path {
			return ops[i].path < ops[j].path
		}
		return order[ops[i].method] < order[ops[j].method]
	})
	for _, op := range ops {
		if _, err := fmt.Fprintf(stdout, "%-6s %s\n", strings.ToUpper(op.method), op.path); err != nil {
			return err
		}
	}
	return nil
}

// printClaims shows the non-identifying claims of a JWT refresh token: its type,
// when it was issued, when it expires, and its scope. Nothing that could be replayed.
func printClaims(w io.Writer, token string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		fmt.Fprintln(w, "  (not a JWT; no claims to show)")
		return
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		fmt.Fprintln(w, "  (payload not decodable)")
		return
	}
	var claims struct {
		Typ   string `json:"typ"`
		Iat   int64  `json:"iat"`
		Exp   int64  `json:"exp"`
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		fmt.Fprintln(w, "  (payload not JSON)")
		return
	}
	fmt.Fprintf(w, "  typ:    %s\n", claims.Typ)
	fmt.Fprintf(w, "  issued: %s\n", unixTime(claims.Iat))
	fmt.Fprintf(w, "  exp:    %s\n", unixTime(claims.Exp))
	fmt.Fprintf(w, "  scope:  %s\n", claims.Scope)
}

func unixTime(seconds int64) string {
	if seconds == 0 {
		return "none"
	}
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}

func refreshTokenFromEnv() (string, error) {
	token := strings.TrimSpace(os.Getenv(envRefreshToken))
	if token == "" {
		return "", fmt.Errorf("%s is not set; run: export %s=\"$(netcupctl login)\"", envRefreshToken, envRefreshToken)
	}
	return token, nil
}

func baseURL() string {
	if v := os.Getenv(envBaseURL); v != "" {
		return v
	}
	return scp.DefaultBaseURL
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func firstN(args []string, n int) []string {
	if len(args) < n {
		return args
	}
	return args[:n]
}

func versionString() string {
	v := version
	if info, ok := debug.ReadBuildInfo(); ok && v == "dev" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				v += "+" + s.Value[:min(12, len(s.Value))]
			}
		}
	}
	return "netcupctl " + v
}
