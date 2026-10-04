// Package credential mints and verifies the short-lived, capability-scoped
// credentials the platform hands to an agent run.
//
// The token is a PASETO v4.public (Ed25519) token. Asymmetric signing is a
// deliberate choice over JWT: there is no in-band algorithm field, so the
// `alg:none` / HS-vs-RS confusion attacks are structurally impossible, and the
// signing key (held only by the minter) is cleanly separable from the
// verification key (used by the tool proxy). The agent worker only ever carries
// the opaque token — it can neither forge nor inspect-and-tamper it.
package credential

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pvec "aidanwoods.dev/go-paseto"
	"github.com/google/uuid"
)

const (
	// Issuer and Audience bind a token to this platform and to the tool proxy.
	Issuer   = "sap-control-plane"
	Audience = "sap-tool-proxy"

	// DefaultTTL is how long a scoped credential is valid. Short by design: a
	// run either finishes within it or the credential is revoked at run end.
	DefaultTTL = 10 * time.Minute
)

// Scoped is the decoded capability carried by a credential.
type Scoped struct {
	JTI          uuid.UUID
	RunID        uuid.UUID
	TenantID     uuid.UUID
	AllowedTools []string
	IssuedAt     time.Time
	NotBefore    time.Time
	ExpiresAt    time.Time
}

// Allows reports whether the given tool is within this credential's scope.
func (s Scoped) Allows(tool string) bool {
	for _, t := range s.AllowedTools {
		if t == tool {
			return true
		}
	}
	return false
}

// Signer mints credentials. Only the control plane holds one.
type Signer struct {
	secret pvec.V4AsymmetricSecretKey
	ttl    time.Duration
}

// Verifier validates credentials with the public key only.
type Verifier struct {
	public pvec.V4AsymmetricPublicKey
}

// NewSignerFromSeedHex builds a Signer from a 32-byte hex-encoded Ed25519 seed
// (as produced by scripts/gen-keys.sh / the PASETO_PRIVATE_KEY env var).
func NewSignerFromSeedHex(seedHex string) (*Signer, error) {
	key, err := pvec.NewV4AsymmetricSecretKeyFromSeed(seedHex)
	if err != nil {
		return nil, fmt.Errorf("credential: parse secret seed: %w", err)
	}
	return &Signer{secret: key, ttl: DefaultTTL}, nil
}

// NewEphemeralSigner generates a throwaway keypair — the zero-config dev default
// when no PASETO_PRIVATE_KEY is set. The control plane both mints and verifies,
// so an in-process keypair is sufficient for local runs.
func NewEphemeralSigner() *Signer {
	return &Signer{secret: pvec.NewV4AsymmetricSecretKey(), ttl: DefaultTTL}
}

// Verifier derives the matching Verifier from this Signer's public key.
func (s *Signer) Verifier() *Verifier {
	return &Verifier{public: s.secret.Public()}
}

// PublicKeyHex returns the hex-encoded public key (useful for logging/config).
func (s *Signer) PublicKeyHex() string { return s.secret.Public().ExportHex() }

// NewVerifierFromHex builds a Verifier from a 32-byte hex-encoded public key.
func NewVerifierFromHex(publicHex string) (*Verifier, error) {
	key, err := pvec.NewV4AsymmetricPublicKeyFromHex(publicHex)
	if err != nil {
		return nil, fmt.Errorf("credential: parse public key: %w", err)
	}
	return &Verifier{public: key}, nil
}

// Mint creates and signs a credential scoped to one run, tenant, and tool set.
// It returns the token string, the decoded Scoped (so the caller can persist the
// jti in the revocation store), and any error.
func (s *Signer) Mint(runID, tenantID uuid.UUID, allowedTools []string, now time.Time) (string, Scoped, error) {
	jti := uuid.New()
	scoped := Scoped{
		JTI:          jti,
		RunID:        runID,
		TenantID:     tenantID,
		AllowedTools: allowedTools,
		IssuedAt:     now,
		NotBefore:    now,
		ExpiresAt:    now.Add(s.ttl),
	}

	toolsJSON, err := json.Marshal(allowedTools)
	if err != nil {
		return "", Scoped{}, fmt.Errorf("credential: encode tools: %w", err)
	}

	t := pvec.NewToken()
	t.SetString("iss", Issuer)
	t.SetString("aud", Audience)
	t.SetString("sub", "run:"+runID.String())
	t.SetString("jti", jti.String())
	t.SetString("run_id", runID.String())
	t.SetString("tenant_id", tenantID.String())
	t.SetString("allowed_tools", string(toolsJSON))
	t.SetTime("iat", scoped.IssuedAt)
	t.SetTime("nbf", scoped.NotBefore)
	t.SetTime("exp", scoped.ExpiresAt)

	return t.V4Sign(s.secret, nil), scoped, nil
}

// ErrInvalidCredential is returned for any signature/claims failure. Callers map
// it to gRPC UNAUTHENTICATED and never leak the underlying reason to the worker.
var ErrInvalidCredential = errors.New("invalid scoped credential")

// Verify checks the signature, audience, issuer, and time bounds, and returns
// the decoded capability. It does NOT consult the revocation store — that is a
// separate, tenant-scoped database check performed by the tool proxy.
func (v *Verifier) Verify(token string, now time.Time) (Scoped, error) {
	p := pvec.NewParser()
	p.AddRule(pvec.NotExpired())
	p.AddRule(pvec.ValidAt(now))
	p.AddRule(pvec.ForAudience(Audience))
	p.AddRule(pvec.IssuedBy(Issuer))

	t, err := p.ParseV4Public(v.public, token, nil)
	if err != nil {
		return Scoped{}, fmt.Errorf("%w: %v", ErrInvalidCredential, err)
	}

	scoped, err := decode(t)
	if err != nil {
		return Scoped{}, fmt.Errorf("%w: %v", ErrInvalidCredential, err)
	}
	return scoped, nil
}

func decode(t *pvec.Token) (Scoped, error) {
	getUUID := func(claim string) (uuid.UUID, error) {
		s, err := t.GetString(claim)
		if err != nil {
			return uuid.Nil, fmt.Errorf("claim %q: %w", claim, err)
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return uuid.Nil, fmt.Errorf("claim %q not a uuid: %w", claim, err)
		}
		return id, nil
	}

	jti, err := getUUID("jti")
	if err != nil {
		return Scoped{}, err
	}
	runID, err := getUUID("run_id")
	if err != nil {
		return Scoped{}, err
	}
	tenantID, err := getUUID("tenant_id")
	if err != nil {
		return Scoped{}, err
	}

	toolsStr, err := t.GetString("allowed_tools")
	if err != nil {
		return Scoped{}, fmt.Errorf("claim allowed_tools: %w", err)
	}
	var tools []string
	if err := json.Unmarshal([]byte(toolsStr), &tools); err != nil {
		return Scoped{}, fmt.Errorf("claim allowed_tools decode: %w", err)
	}

	iat, _ := t.GetTime("iat")
	nbf, _ := t.GetTime("nbf")
	exp, _ := t.GetTime("exp")

	return Scoped{
		JTI: jti, RunID: runID, TenantID: tenantID,
		AllowedTools: tools, IssuedAt: iat, NotBefore: nbf, ExpiresAt: exp,
	}, nil
}
