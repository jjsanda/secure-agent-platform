package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/devseed"
)

// Token lifetimes. Access and ID tokens are short-lived RS256 JWTs; the opaque
// refresh token is longer-lived and rotated on every use.
const (
	accessTokenTTL  = 15 * time.Minute
	idTokenTTL      = 15 * time.Minute
	authCodeTTL     = 2 * time.Minute
	refreshTokenTTL = 8 * time.Hour
)

// signingKey bundles the RSA private key with the stable key id (`kid`) that is
// advertised in the JWKS and stamped into every token header, so verifiers can
// select the right key.
type signingKey struct {
	private *rsa.PrivateKey
	kid     string
}

// newSigningKey generates a fresh 2048-bit RSA key and derives a stable kid from
// the RFC 7638 JWK thumbprint of the public key. The kid is therefore
// content-addressed: identical keys always yield the same id.
func newSigningKey() (*signingKey, error) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate rsa key: %w", err)
	}
	return &signingKey{private: private, kid: jwkThumbprint(&private.PublicKey)}, nil
}

// jsonWebKey is a single RSA public key in JWK form (RFC 7517).
type jsonWebKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwks renders the public key as a JWK Set body served at the jwks_uri.
func (k *signingKey) jwks() map[string]any {
	pub := k.private.PublicKey
	return map[string]any{
		"keys": []jsonWebKey{{
			Kty: "RSA",
			Use: "sig",
			Alg: "RS256",
			Kid: k.kid,
			N:   b64(pub.N.Bytes()),
			E:   b64(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
}

// mintAccessToken builds the RS256 access token. Its audience is the API
// (sap-control-plane); the control plane authorizes requests off these claims,
// and tenant_id in particular is projected into the Postgres RLS GUC.
func (k *signingKey) mintAccessToken(u devseed.User, issuer, audience string, now time.Time) (string, error) {
	return k.sign(jwt.MapClaims{
		"iss":       issuer,
		"sub":       u.Subject,
		"aud":       audience,
		"iat":       now.Unix(),
		"exp":       now.Add(accessTokenTTL).Unix(),
		"tenant_id": u.TenantID,
		"email":     u.Email,
		"name":      u.Name,
		"roles":     u.Roles,
	})
}

// mintIDToken builds the RS256 ID token. Its audience is the SPA client
// (sap-dashboard) and it echoes the login nonce when one was supplied, per OIDC.
func (k *signingKey) mintIDToken(u devseed.User, issuer, clientID, nonce string, now time.Time) (string, error) {
	claims := jwt.MapClaims{
		"iss":       issuer,
		"sub":       u.Subject,
		"aud":       clientID,
		"iat":       now.Unix(),
		"exp":       now.Add(idTokenTTL).Unix(),
		"tenant_id": u.TenantID,
		"email":     u.Email,
		"name":      u.Name,
		"roles":     u.Roles,
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	return k.sign(claims)
}

// sign serializes claims as an RS256 JWT with the key id in the header.
func (k *signingKey) sign(claims jwt.MapClaims) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = k.kid
	return tok.SignedString(k.private)
}

// verifyPKCE checks a code_verifier against a stored S256 code_challenge
// (RFC 7636 §4.6): base64url(sha256(verifier)) must equal the challenge. The
// derived challenge is compared in constant time.
func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	derived := b64(sum[:])
	return subtle.ConstantTimeCompare([]byte(derived), []byte(challenge)) == 1
}

// jwkThumbprint computes the RFC 7638 SHA-256 thumbprint of an RSA public key.
func jwkThumbprint(pub *rsa.PublicKey) string {
	// Canonical JWK: required members only, lexicographic order, no whitespace.
	canonical := fmt.Sprintf(`{"e":%q,"kty":"RSA","n":%q}`,
		b64(big.NewInt(int64(pub.E)).Bytes()), b64(pub.N.Bytes()))
	sum := sha256.Sum256([]byte(canonical))
	return b64(sum[:])
}

// newOpaqueToken returns a cryptographically random, URL-safe opaque token used
// for authorization codes and refresh tokens (256 bits of entropy).
func newOpaqueToken() string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand should never fail; if it does the process cannot safely continue.
		panic(fmt.Sprintf("mock-oidc: crypto/rand failed: %v", err))
	}
	return b64(buf[:])
}

// b64 is base64url without padding, the encoding JOSE uses everywhere.
func b64(p []byte) string { return base64.RawURLEncoding.EncodeToString(p) }
