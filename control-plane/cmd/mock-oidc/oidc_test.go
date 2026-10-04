package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/devseed"
)

// startTestServer boots the provider on a loopback port and returns its issuer.
//
// The issuer is set to the actual bound address. This matters because go-oidc's
// discovery (oidc.NewProvider) requires the discovery document's `issuer` to
// equal the URL it was fetched from — so we cannot use the localhost:9000
// default while listening on a random test port.
func startTestServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	issuer := "http://" + ln.Addr().String()
	srv, err := newServer(config{
		addr:        ln.Addr().String(),
		issuer:      issuer,
		clientID:    "sap-dashboard",
		apiAudience: "sap-control-plane",
	})
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	hs := &http.Server{Handler: srv.mux}
	go func() { _ = hs.Serve(ln) }()
	t.Cleanup(func() { _ = hs.Close() })
	return issuer
}

// TestAuthorizationCodePKCEFlow drives the full auth-code + PKCE flow in-process
// and then verifies the returned tokens with github.com/coreos/go-oidc — proving
// this provider is a real, RS256-signing, go-oidc-compatible IdP. It also asserts
// refresh-token rotation and single-use authorization codes.
func TestAuthorizationCodePKCEFlow(t *testing.T) {
	issuer := startTestServer(t)

	// PKCE: a high-entropy verifier and its S256 challenge (RFC 7636).
	verifier := newOpaqueToken()
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	const (
		redirectURI = "http://localhost:5173/callback"
		state       = "state-123"
		nonce       = "nonce-abc"
	)

	// --- /authorize: scripted login=alice, capture the code from the redirect ---
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	authz := url.Values{
		"response_type":         {"code"},
		"client_id":             {"sap-dashboard"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"login":                 {"alice"},
	}
	resp, err := noRedirect.Get(issuer + "/authorize?" + authz.Encode())
	if err != nil {
		t.Fatalf("GET /authorize: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d, want 302", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect Location: %v", err)
	}
	if got := loc.Query().Get("state"); got != state {
		t.Fatalf("redirect state = %q, want %q", got, state)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("no authorization code in redirect")
	}

	// --- /token: exchange the code with the PKCE verifier ---
	exchange := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {"sap-dashboard"},
		"code_verifier": {verifier},
	}
	tok := mustPostToken(t, issuer, exchange)
	if tok.AccessToken == "" || tok.IDToken == "" || tok.RefreshToken == "" {
		t.Fatalf("missing tokens in response: %+v", tok)
	}
	if tok.TokenType != "Bearer" {
		t.Fatalf("token_type = %q, want Bearer", tok.TokenType)
	}
	if tok.ExpiresIn <= 0 {
		t.Fatalf("expires_in = %d, want > 0", tok.ExpiresIn)
	}

	// --- go-oidc verification: this is the whole point of the exercise ---
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, issuer) // discovery + issuer check
	if err != nil {
		t.Fatalf("oidc.NewProvider: %v", err)
	}

	// Access token: audience is the API. Verify() checks the RS256 signature
	// against the JWKS from discovery, plus iss / aud / exp.
	accessVerifier := provider.Verifier(&oidc.Config{
		ClientID:             "sap-control-plane",
		SupportedSigningAlgs: []string{"RS256"},
	})
	at, err := accessVerifier.Verify(ctx, tok.AccessToken)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if at.Subject != "alice" {
		t.Fatalf("access token sub = %q, want alice", at.Subject)
	}
	var claims struct {
		TenantID string   `json:"tenant_id"`
		Email    string   `json:"email"`
		Name     string   `json:"name"`
		Roles    []string `json:"roles"`
	}
	if err := at.Claims(&claims); err != nil {
		t.Fatalf("decode access-token claims: %v", err)
	}
	if claims.TenantID != devseed.TenantAID {
		t.Fatalf("tenant_id = %q, want %q", claims.TenantID, devseed.TenantAID)
	}
	if claims.Email != "alice@tenant-a.example" {
		t.Fatalf("email = %q, want alice@tenant-a.example", claims.Email)
	}

	// Prove the audience is actually enforced: a verifier expecting a different
	// audience must reject the same token.
	wrongAud := provider.Verifier(&oidc.Config{
		ClientID:             "someone-else",
		SupportedSigningAlgs: []string{"RS256"},
	})
	if _, err := wrongAud.Verify(ctx, tok.AccessToken); err == nil {
		t.Fatal("expected audience mismatch to fail verification, but it passed")
	}

	// ID token: audience is the SPA client and the login nonce is echoed back.
	idVerifier := provider.Verifier(&oidc.Config{
		ClientID:             "sap-dashboard",
		SupportedSigningAlgs: []string{"RS256"},
	})
	idt, err := idVerifier.Verify(ctx, tok.IDToken)
	if err != nil {
		t.Fatalf("verify id token: %v", err)
	}
	if idt.Subject != "alice" {
		t.Fatalf("id token sub = %q, want alice", idt.Subject)
	}
	if idt.Nonce != nonce {
		t.Fatalf("id token nonce = %q, want %q", idt.Nonce, nonce)
	}

	// --- refresh-token rotation: old token dies, a new (different) one is issued ---
	refreshed := mustPostToken(t, issuer, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
		"client_id":     {"sap-dashboard"},
	})
	if refreshed.RefreshToken == "" || refreshed.RefreshToken == tok.RefreshToken {
		t.Fatalf("refresh token not rotated: old=%q new=%q", tok.RefreshToken, refreshed.RefreshToken)
	}
	if _, err := accessVerifier.Verify(ctx, refreshed.AccessToken); err != nil {
		t.Fatalf("verify refreshed access token: %v", err)
	}
	// The old refresh token must now be rejected (it was rotated out).
	if status, body := postToken(t, issuer, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
	}); status != http.StatusBadRequest || !strings.Contains(body, "invalid_grant") {
		t.Fatalf("reusing rotated refresh token: status=%d body=%s, want 400 invalid_grant", status, body)
	}

	// --- single-use: replaying the consumed authorization code must fail ---
	if status, body := postToken(t, issuer, exchange); status != http.StatusBadRequest || !strings.Contains(body, "invalid_grant") {
		t.Fatalf("replaying auth code: status=%d body=%s, want 400 invalid_grant", status, body)
	}
}

// TestAuthorizeRejectsMissingPKCE asserts a non-S256 request is bounced back to
// the redirect_uri as an invalid_request error rather than issuing a code.
func TestAuthorizeRejectsMissingPKCE(t *testing.T) {
	issuer := startTestServer(t)
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	authz := url.Values{
		"response_type": {"code"},
		"client_id":     {"sap-dashboard"},
		"redirect_uri":  {"http://localhost:5173/callback"},
		"scope":         {"openid"},
		"login":         {"alice"},
		// no code_challenge / code_challenge_method
	}
	resp, err := noRedirect.Get(issuer + "/authorize?" + authz.Encode())
	if err != nil {
		t.Fatalf("GET /authorize: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 error redirect", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got := loc.Query().Get("error"); got != "invalid_request" {
		t.Fatalf("error = %q, want invalid_request", got)
	}
	if loc.Query().Get("code") != "" {
		t.Fatal("an authorization code was issued despite missing PKCE")
	}
}

// TestAuthorizeLoginPicker asserts the HTML picker is rendered (and lists the
// demo users) when no user has been selected.
func TestAuthorizeLoginPicker(t *testing.T) {
	issuer := startTestServer(t)
	authz := url.Values{
		"response_type":         {"code"},
		"client_id":             {"sap-dashboard"},
		"redirect_uri":          {"http://localhost:5173/callback"},
		"scope":                 {"openid"},
		"code_challenge":        {"x"},
		"code_challenge_method": {"S256"},
		// no login -> picker
	}
	resp, err := http.Get(issuer + "/authorize?" + authz.Encode())
	if err != nil {
		t.Fatalf("GET /authorize: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, u := range devseed.Users {
		if !strings.Contains(string(body), u.Subject) {
			t.Fatalf("login picker missing user %q", u.Subject)
		}
	}
}

// mustPostToken posts to /token and requires a 200 with a decodable body.
func mustPostToken(t *testing.T, issuer string, form url.Values) tokenResponse {
	t.Helper()
	status, body := postToken(t, issuer, form)
	if status != http.StatusOK {
		t.Fatalf("POST /token status = %d, body = %s", status, body)
	}
	var tr tokenResponse
	if err := json.Unmarshal([]byte(body), &tr); err != nil {
		t.Fatalf("decode token response: %v (body=%s)", err, body)
	}
	return tr
}

// postToken posts a form to /token and returns the status and raw body.
func postToken(t *testing.T, issuer string, form url.Values) (int, string) {
	t.Helper()
	resp, err := http.PostForm(issuer+"/token", form)
	if err != nil {
		t.Fatalf("POST /token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}
