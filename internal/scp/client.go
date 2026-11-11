package scp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// maxBody caps how much of a response the client reads (the OpenAPI document is
// the largest, at a few megabytes).
const maxBody = 32 << 20

// Client calls the SCP REST API with access tokens from a TokenSource.
type Client struct {
	baseURL   string
	http      *http.Client
	userAgent string
}

// NewClient returns a client for the API under baseURL. base, if not nil, is the
// transport-level client (timeouts, test servers); tokens are added on top of it.
func NewClient(ctx context.Context, baseURL string, base *http.Client, tokens oauth2.TokenSource, userAgent string) *Client {
	if base == nil {
		base = &http.Client{Timeout: 60 * time.Second}
	}
	authed := oauth2.NewClient(withHTTPClient(ctx, base), tokens)
	authed.Timeout = base.Timeout
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/") + apiPath,
		http:      authed,
		userAgent: userAgent,
	}
}

// APIError is a non-2xx answer. It carries the status and a bounded excerpt of
// the body, never request headers.
type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// GetRaw performs a GET on path (relative to /scp-core/api/v1) and returns the body.
func (c *Client) GetRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("GET %s: read body: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{Method: http.MethodGet, Path: path, Status: resp.StatusCode, Body: excerpt(body)}
	}
	return body, nil
}

func excerpt(body []byte) string {
	const limit = 512
	s := strings.TrimSpace(string(body))
	if len(s) > limit {
		s = s[:limit] + "…"
	}
	return s
}
