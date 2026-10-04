package main

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/devseed"
)

// config is the resolved mock-oidc configuration (see configFromEnv).
type config struct {
	addr        string // listen address
	issuer      string // external issuer URL; base of all endpoint URLs and the `iss` claim
	clientID    string // the public SPA client and the ID-token audience
	apiAudience string // the access-token audience the control plane checks
}

// server holds the signing key, in-memory stores and routing for the provider.
type server struct {
	cfg    config
	key    *signingKey
	codes  *memStore[authCode]
	tokens *memStore[refreshRecord]
	mux    *http.ServeMux
	log    *slog.Logger
}

// newServer generates the signing key and wires up the routes.
func newServer(cfg config) (*server, error) {
	key, err := newSigningKey()
	if err != nil {
		return nil, err
	}
	s := &server{
		cfg:    cfg,
		key:    key,
		codes:  newMemStore[authCode](),
		tokens: newMemStore[refreshRecord](),
		mux:    http.NewServeMux(),
		log:    slog.Default(),
	}
	s.routes()
	return s, nil
}

func (s *server) routes() {
	s.mux.HandleFunc("/.well-known/openid-configuration", s.handleDiscovery)
	s.mux.HandleFunc("/jwks", s.handleJWKS)
	s.mux.HandleFunc("/authorize", s.handleAuthorize)
	s.mux.HandleFunc("/token", s.handleToken)
	s.mux.HandleFunc("/userinfo", s.handleUserinfo)
	s.mux.HandleFunc("/logout", s.handleLogout)
	s.mux.HandleFunc("/revoke", s.handleRevoke)
}

// handleDiscovery serves the OIDC discovery document. All URLs are derived from
// the configured issuer so the document is internally consistent regardless of
// whether it is fetched from localhost (browser) or the compose network name.
func (s *server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	iss := s.cfg.issuer
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                iss,
		"authorization_endpoint":                iss + "/authorize",
		"token_endpoint":                        iss + "/token",
		"jwks_uri":                              iss + "/jwks",
		"userinfo_endpoint":                     iss + "/userinfo",
		"end_session_endpoint":                  iss + "/logout",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"}, // public client, PKCE only
		"scopes_supported":                      []string{"openid", "profile", "email"},
		"claims_supported":                      []string{"sub", "tenant_id", "email", "name", "roles", "iss", "aud", "exp", "iat"},
	})
}

// handleJWKS serves the single RSA signing key as a JWK Set.
func (s *server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	writeJSON(w, http.StatusOK, s.key.jwks())
}

// handleAuthorize implements the authorization-code + PKCE endpoint.
//
// With no user selected it renders a login picker listing the demo users. It
// also honours a scripted `login=<subject>` parameter so the whole flow can be
// driven headlessly from curl or a test. On success it issues a single-use code
// and 302-redirects to redirect_uri?code=...&state=...
func (s *server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	q := r.Form
	redirectURI := q.Get("redirect_uri")
	clientID := q.Get("client_id")
	state := q.Get("state")

	// redirect_uri and client_id are validated BEFORE any error is redirected:
	// bouncing error details to an unvalidated destination would be an open
	// redirect. This mock has one registered client and accepts any absolute
	// redirect_uri for it (a real IdP matches against registered URIs).
	redirect, err := url.Parse(redirectURI)
	if redirectURI == "" || err != nil || !redirect.IsAbs() {
		http.Error(w, "invalid or missing redirect_uri", http.StatusBadRequest)
		return
	}
	if clientID != s.cfg.clientID {
		http.Error(w, "unknown client_id", http.StatusBadRequest)
		return
	}

	if rt := q.Get("response_type"); rt != "code" {
		redirectError(w, r, redirect, state, "unsupported_response_type", "only response_type=code is supported")
		return
	}
	// PKCE is mandatory and only S256 is accepted.
	if m := q.Get("code_challenge_method"); m != "S256" {
		redirectError(w, r, redirect, state, "invalid_request", "code_challenge_method must be S256")
		return
	}
	challenge := q.Get("code_challenge")
	if challenge == "" {
		redirectError(w, r, redirect, state, "invalid_request", "code_challenge is required")
		return
	}

	scope := q.Get("scope")
	if scope == "" {
		scope = "openid profile email"
	}

	login := q.Get("login")
	if login == "" {
		// No user chosen yet: render the picker, preserving the request params.
		s.renderLoginPicker(w, q)
		return
	}
	user, ok := devseed.UserBySubject(login)
	if !ok {
		redirectError(w, r, redirect, state, "access_denied", "unknown user")
		return
	}

	code := newOpaqueToken()
	s.codes.put(code, authCode{
		subject:       user.Subject,
		codeChallenge: challenge,
		redirectURI:   redirectURI,
		nonce:         q.Get("nonce"),
		clientID:      clientID,
		scope:         scope,
		expiry:        time.Now().Add(authCodeTTL),
	})

	out := *redirect
	params := out.Query()
	params.Set("code", code)
	if state != "" {
		params.Set("state", state)
	}
	out.RawQuery = params.Encode()
	http.Redirect(w, r, out.String(), http.StatusFound)
}

// handleToken implements the OAuth2 token endpoint for the two supported grants.
func (s *server) handleToken(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	if r.Method == http.MethodOptions { // CORS preflight from the browser SPA
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		tokenError(w, http.StatusMethodNotAllowed, "invalid_request", "POST required")
		return
	}
	if err := r.ParseForm(); err != nil {
		tokenError(w, http.StatusBadRequest, "invalid_request", "malformed form body")
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		s.grantAuthorizationCode(w, r)
	case "refresh_token":
		s.grantRefreshToken(w, r)
	default:
		tokenError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

// grantAuthorizationCode redeems a code for tokens after validating PKCE.
func (s *server) grantAuthorizationCode(w http.ResponseWriter, r *http.Request) {
	f := r.PostForm
	// Consume the code up front so it is single-use even if validation fails.
	ac, ok := s.codes.consume(f.Get("code"))
	if !ok {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "unknown or already-used authorization code")
		return
	}
	if time.Now().After(ac.expiry) {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "authorization code expired")
		return
	}
	if f.Get("redirect_uri") != ac.redirectURI {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}
	if cid := f.Get("client_id"); cid != "" && cid != ac.clientID {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}
	if !verifyPKCE(f.Get("code_verifier"), ac.codeChallenge) {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}
	user, ok := devseed.UserBySubject(ac.subject)
	if !ok {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "unknown subject")
		return
	}
	s.issueTokens(w, user, ac.nonce, ac.scope)
}

// grantRefreshToken rotates the presented refresh token and mints fresh tokens.
func (s *server) grantRefreshToken(w http.ResponseWriter, r *http.Request) {
	// Rotation: consume (invalidate) the presented token; issueTokens mints a new one.
	old, ok := s.tokens.consume(r.PostForm.Get("refresh_token"))
	if !ok {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "unknown or already-rotated refresh token")
		return
	}
	if time.Now().After(old.expiry) {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "refresh token expired")
		return
	}
	user, ok := devseed.UserBySubject(old.subject)
	if !ok {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "unknown subject")
		return
	}
	s.issueTokens(w, user, old.nonce, old.scope)
}

// tokenResponse is the RFC 6749 §5.1 successful token response.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// issueTokens mints the access + ID JWTs and a rotated opaque refresh token.
func (s *server) issueTokens(w http.ResponseWriter, user devseed.User, nonce, scope string) {
	now := time.Now()
	access, err := s.key.mintAccessToken(user, s.cfg.issuer, s.cfg.apiAudience, now)
	if err != nil {
		s.log.Error("sign access token", "err", err)
		tokenError(w, http.StatusInternalServerError, "server_error", "could not sign access token")
		return
	}
	idToken, err := s.key.mintIDToken(user, s.cfg.issuer, s.cfg.clientID, nonce, now)
	if err != nil {
		s.log.Error("sign id token", "err", err)
		tokenError(w, http.StatusInternalServerError, "server_error", "could not sign id token")
		return
	}
	refresh := newOpaqueToken()
	s.tokens.put(refresh, refreshRecord{
		subject: user.Subject,
		scope:   scope,
		nonce:   nonce,
		expiry:  now.Add(refreshTokenTTL),
	})

	w.Header().Set("Cache-Control", "no-store") // tokens must never be cached
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		IDToken:      idToken,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTokenTTL.Seconds()),
		Scope:        scope,
	})
}

// handleUserinfo returns the standard/custom claims for a valid access token.
// It keeps the discovery document honest (userinfo_endpoint is advertised).
func (s *server) handleUserinfo(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	raw, ok := bearerToken(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_request"`)
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return &s.key.private.PublicKey, nil
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(s.cfg.issuer),
		jwt.WithAudience(s.cfg.apiAudience),
	)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sub":       claims["sub"],
		"tenant_id": claims["tenant_id"],
		"email":     claims["email"],
		"name":      claims["name"],
		"roles":     claims["roles"],
	})
}

// handleLogout is a best-effort RP-initiated logout (end_session_endpoint).
// Tokens are self-contained so there is no server-side session to clear; it just
// bounces back to post_logout_redirect_uri when one is supplied.
func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	_ = r.ParseForm()
	if target := r.Form.Get("post_logout_redirect_uri"); target != "" {
		if u, err := url.Parse(target); err == nil && u.IsAbs() {
			if state := r.Form.Get("state"); state != "" {
				params := u.Query()
				params.Set("state", state)
				u.RawQuery = params.Encode()
			}
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("logged out"))
}

// handleRevoke is best-effort token revocation (RFC 7009). Only opaque refresh
// tokens are stateful and revocable here; access/ID tokens are stateless JWTs
// that simply expire. Always returns 200, even for an unknown token.
func (s *server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_ = r.ParseForm()
	if tok := r.PostForm.Get("token"); tok != "" {
		s.tokens.delete(tok)
	}
	w.WriteHeader(http.StatusOK)
}

// --- rendering & helpers ---------------------------------------------------

// loginPicker is the minimal HTML shown when no user has been selected. Each
// button re-submits the same authorization request with login=<subject> set.
var loginPicker = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>mock-oidc — sign in</title>
<style>
 body{font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem}
 h1{font-size:1.25rem}
 .sub{color:#666;font-size:.85rem}
 button{display:block;width:100%;padding:.75rem 1rem;margin:.5rem 0;font-size:1rem;
   text-align:left;border:1px solid #ccc;border-radius:.5rem;background:#fafafa;cursor:pointer}
 button:hover{background:#eef}
</style>
</head>
<body>
<h1>mock-oidc</h1>
<p class="sub">Dev-only identity provider. Pick a demo user to sign in:</p>
<form method="get" action="/authorize">
{{range .Hidden}}<input type="hidden" name="{{.Name}}" value="{{.Value}}">
{{end}}{{range .Users}}<button type="submit" name="login" value="{{.Subject}}">
{{.Name}}<br><span class="sub">{{.Email}} · {{.Subject}}</span>
</button>
{{end}}</form>
</body>
</html>`))

type nameValue struct{ Name, Value string }

type pickerData struct {
	Hidden []nameValue
	Users  []devseed.User
}

// renderLoginPicker writes the picker, carrying every original request parameter
// through as a hidden field so the resubmission is a valid authorization request.
func (s *server) renderLoginPicker(w http.ResponseWriter, q url.Values) {
	var hidden []nameValue
	for _, name := range []string{
		"response_type", "client_id", "redirect_uri", "scope",
		"state", "nonce", "code_challenge", "code_challenge_method",
	} {
		if v := q.Get(name); v != "" {
			hidden = append(hidden, nameValue{Name: name, Value: v})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := loginPicker.Execute(w, pickerData{Hidden: hidden, Users: devseed.Users}); err != nil {
		s.log.Error("render login picker", "err", err)
	}
}

// redirectError bounces an OAuth2 error back to the client's redirect_uri
// (RFC 6749 §4.1.2.1).
func redirectError(w http.ResponseWriter, r *http.Request, redirect *url.URL, state, code, desc string) {
	out := *redirect
	params := out.Query()
	params.Set("error", code)
	params.Set("error_description", desc)
	if state != "" {
		params.Set("state", state)
	}
	out.RawQuery = params.Encode()
	http.Redirect(w, r, out.String(), http.StatusFound)
}

// bearerToken extracts a token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return h[len(prefix):], true
	}
	return "", false
}

// setCORS applies permissive CORS for the browser SPA (origin
// http://localhost:5173). No credentials are used — tokens travel in the
// Authorization header or request body — so reflecting the requesting origin,
// or "*" for non-browser callers, is safe.
func setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Add("Vary", "Origin")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	h.Set("Access-Control-Max-Age", "3600")
}

// writeJSON encodes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// tokenError writes an RFC 6749 §5.2 OAuth2 error response.
func tokenError(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]string{
		"error":             code,
		"error_description": desc,
	})
}
