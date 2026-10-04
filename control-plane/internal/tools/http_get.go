package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/guard"
)

// HTTPGet fetches a URL over HTTP(S). It is the canonical example of a tool that
// needs guarding: an unguarded fetch tool is a server-side request forgery
// primitive. The policy guard blocks private/loopback/metadata targets, and the
// connection is pinned to the validated IP so DNS cannot be rebound between the
// check and the dial.
type HTTPGet struct{}

// Name implements Tool.
func (HTTPGet) Name() string { return "http_get" }

// Validate implements Tool: url must be a non-empty string, and it must pass the
// SSRF guard (public, routable host) before the call is even authorized.
func (HTTPGet) Validate(args map[string]any) error {
	u, ok := args["url"].(string)
	if !ok || u == "" {
		return fmt.Errorf("%w: http_get requires a string %q", ErrInvalidArgs, "url")
	}
	if _, err := guard.CheckPublicURL(u); err != nil {
		return err // guard.ErrBlocked -> POLICY_BLOCKED
	}
	return nil
}

const httpGetMaxBody = 64 * 1024

// Execute implements Tool.
func (HTTPGet) Execute(ctx context.Context, args map[string]any) (map[string]any, error) {
	rawURL, ok := args["url"].(string)
	if !ok || rawURL == "" {
		return nil, fmt.Errorf("http_get: missing required argument %q", "url")
	}

	ips, err := guard.CheckPublicURL(rawURL)
	if err != nil {
		return nil, err // guard.ErrBlocked -> POLICY_BLOCKED at the proxy
	}
	pinned := ips[0]

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				_, port, splitErr := net.SplitHostPort(addr)
				if splitErr != nil {
					return nil, splitErr
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(pinned.String(), port))
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if _, err := guard.CheckPublicURL(req.URL.String()); err != nil {
				return err
			}
			if len(via) >= 3 {
				return fmt.Errorf("http_get: too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("http_get: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http_get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, httpGetMaxBody))
	return map[string]any{
		"status": resp.StatusCode,
		"bytes":  len(body),
		"body":   string(body),
	}, nil
}
