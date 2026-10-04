package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Verifier validates OIDC access tokens against a provider's JWKS and maps them
// to Claims. It is provider-agnostic: pointing OIDC_ISSUER_URL at Keycloak or
// the bundled mock is the only difference.
type Verifier struct {
	oidc *oidc.IDTokenVerifier
}

// NewVerifier builds a token verifier.
//
// issuerURL is the issuer as it appears in a token's `iss` claim (and as the
// browser reaches the IdP). discoveryURL is where the control plane actually
// fetches OIDC metadata + JWKS from.
//
//   - When they are equal (the normal case, e.g. Keycloak reachable at one URL),
//     we use standard discovery.
//   - When they differ (Docker: the browser sees http://localhost:9000 while the
//     backend sees http://mock-oidc:9000), the token's issuer would never match
//     the discovery URL, and the discovery document's jwks_uri would point at the
//     browser host, which the backend cannot reach. We therefore fetch discovery
//     from discoveryURL, keep the token issuer as issuerURL, and repoint the JWKS
//     key set at the reachable host (preserving the jwks path from discovery, so
//     this stays provider-agnostic).
func NewVerifier(ctx context.Context, issuerURL, discoveryURL, audience string) (*Verifier, error) {
	cfg := &oidc.Config{ClientID: audience}

	if discoveryURL == "" || discoveryURL == issuerURL {
		provider, err := discoverWithRetry(ctx, issuerURL)
		if err != nil {
			return nil, err
		}
		return &Verifier{oidc: provider.Verifier(cfg)}, nil
	}

	keySet, err := splitKeySet(ctx, issuerURL, discoveryURL)
	if err != nil {
		return nil, err
	}
	return &Verifier{oidc: oidc.NewVerifier(issuerURL, keySet, cfg)}, nil
}

func discoverWithRetry(ctx context.Context, issuerURL string) (*oidc.Provider, error) {
	var last error
	for range 30 {
		p, err := oidc.NewProvider(ctx, issuerURL)
		if err == nil {
			return p, nil
		}
		last = err
		if err := sleep(ctx, 2*time.Second); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("auth: OIDC discovery at %s failed after retries: %w", issuerURL, last)
}

// splitKeySet handles the Docker issuer/host split described on NewVerifier.
func splitKeySet(ctx context.Context, issuerURL, discoveryURL string) (oidc.KeySet, error) {
	type doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	var meta doc
	var last error
	discovery := strings.TrimRight(discoveryURL, "/") + "/.well-known/openid-configuration"
	for range 30 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, discovery, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			meta = doc{}
			err = json.NewDecoder(resp.Body).Decode(&meta)
			_ = resp.Body.Close()
			if err == nil && meta.JWKSURI != "" {
				break
			}
		}
		last = err
		if err := sleep(ctx, 2*time.Second); err != nil {
			return nil, err
		}
	}
	if meta.JWKSURI == "" {
		return nil, fmt.Errorf("auth: OIDC discovery at %s failed after retries: %w", discovery, last)
	}
	if meta.Issuer != issuerURL {
		return nil, fmt.Errorf("auth: discovery issuer %q does not match expected %q", meta.Issuer, issuerURL)
	}
	jwks, err := repointHost(meta.JWKSURI, discoveryURL)
	if err != nil {
		return nil, err
	}
	return oidc.NewRemoteKeySet(ctx, jwks), nil
}

// repointHost rewrites rawURL's scheme+host to those of hostSource, keeping its path.
func repointHost(rawURL, hostSource string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("auth: parse jwks_uri: %w", err)
	}
	h, err := url.Parse(hostSource)
	if err != nil {
		return "", fmt.Errorf("auth: parse discovery url: %w", err)
	}
	u.Scheme, u.Host = h.Scheme, h.Host
	return u.String(), nil
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// verifyToken validates a raw access token (signature via JWKS, audience, expiry)
// and returns the mapped Claims.
func (v *Verifier) verifyToken(ctx context.Context, raw string) (Claims, error) {
	tok, err := v.oidc.Verify(ctx, raw)
	if err != nil {
		return Claims{}, err
	}
	var rc rawClaims
	if err := tok.Claims(&rc); err != nil {
		return Claims{}, fmt.Errorf("auth: decode claims: %w", err)
	}
	return rc.toClaims(), nil
}

// rawClaims accepts both the mock IdP shape (top-level `roles`) and Keycloak's
// (`realm_access.roles`), so the control plane needs no provider-specific branch.
type rawClaims struct {
	Subject     string   `json:"sub"`
	TenantID    string   `json:"tenant_id"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	Issuer      string   `json:"iss"`
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

func (r rawClaims) toClaims() Claims {
	roles := make([]string, 0, len(r.Roles)+len(r.RealmAccess.Roles))
	roles = append(roles, r.Roles...)
	roles = append(roles, r.RealmAccess.Roles...)
	return Claims{
		Subject:  r.Subject,
		TenantID: r.TenantID,
		Email:    r.Email,
		Name:     r.Name,
		Roles:    dedupe(roles),
		Issuer:   r.Issuer,
	}
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
