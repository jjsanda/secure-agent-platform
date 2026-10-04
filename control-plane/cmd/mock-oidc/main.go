// Command mock-oidc is a tiny, standards-compliant OIDC / OAuth2 provider that
// serves as the zero-dependency default identity provider for the
// secure-agent-platform dev stack. It is a drop-in alternative to Keycloak so
// the whole platform can run OIDC login with no external service.
//
// It implements the authorization-code + PKCE (S256) flow with real RS256
// signing and exposes:
//
//	GET  /.well-known/openid-configuration   discovery document
//	GET  /jwks                               JWK Set (one RSA signing key)
//	GET  /authorize                          login picker + code issuance (PKCE)
//	POST /token                              authorization_code & refresh_token grants
//	GET  /userinfo                           claims for a valid access token
//	GET|POST /logout                         end_session_endpoint (best-effort)
//	POST /revoke                             token revocation (best-effort)
//
// The custom `tenant_id` claim ties an issued token to a seeded demo tenant
// (see internal/devseed): the control plane projects it into a Postgres RLS GUC,
// so it is the linchpin of tenant isolation.
//
// This is a DEV-ONLY identity provider. It has no credential store (users are
// picked, not authenticated), keeps all state in memory, and mints a fresh
// signing key on every boot. Never use it in production.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := configFromEnv()
	srv, err := newServer(cfg)
	if err != nil {
		logger.Error("mock-oidc failed to start", "err", err)
		os.Exit(1)
	}

	logger.Info("mock-oidc listening",
		"addr", cfg.addr,
		"issuer", cfg.issuer,
		"client_id", cfg.clientID,
		"api_audience", cfg.apiAudience,
		"kid", srv.key.kid,
	)

	httpServer := &http.Server{
		Addr:              cfg.addr,
		Handler:           srv.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil {
		logger.Error("mock-oidc server stopped", "err", err)
		os.Exit(1)
	}
}

// configFromEnv resolves configuration from the environment, applying the
// zero-config dev defaults documented on each variable.
func configFromEnv() config {
	return config{
		addr:        env("MOCK_OIDC_ADDR", ":9000"),
		issuer:      strings.TrimRight(env("MOCK_OIDC_ISSUER", "http://localhost:9000"), "/"),
		clientID:    env("MOCK_OIDC_CLIENT_ID", "sap-dashboard"),
		apiAudience: env("MOCK_OIDC_API_AUDIENCE", "sap-control-plane"),
	}
}

// env returns the value of key, or def when unset/empty.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
