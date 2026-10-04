package credential

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMintVerifyRoundTrip(t *testing.T) {
	s := NewEphemeralSigner()
	v := s.Verifier()
	now := time.Now()
	runID, tenantID := uuid.New(), uuid.New()

	tok, scoped, err := s.Mint(runID, tenantID, []string{"echo", "read_doc"}, now)
	if err != nil {
		t.Fatal(err)
	}

	got, err := v.Verify(tok, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.RunID != runID || got.TenantID != tenantID || got.JTI != scoped.JTI {
		t.Fatalf("claims mismatch: %+v", got)
	}
	if !got.Allows("echo") || !got.Allows("read_doc") || got.Allows("delete_all") {
		t.Fatalf("scope wrong: %v", got.AllowedTools)
	}
}

func TestExpiredRejected(t *testing.T) {
	s := NewEphemeralSigner()
	now := time.Now()
	tok, _, _ := s.Mint(uuid.New(), uuid.New(), []string{"echo"}, now)
	if _, err := s.Verifier().Verify(tok, now.Add(DefaultTTL+time.Minute)); err == nil {
		t.Fatal("expected expired credential to be rejected")
	}
}

func TestNotYetValidRejected(t *testing.T) {
	s := NewEphemeralSigner()
	now := time.Now()
	tok, _, _ := s.Mint(uuid.New(), uuid.New(), []string{"echo"}, now)
	if _, err := s.Verifier().Verify(tok, now.Add(-time.Minute)); err == nil {
		t.Fatal("expected not-yet-valid credential to be rejected")
	}
}

func TestWrongKeyRejected(t *testing.T) {
	s1, s2 := NewEphemeralSigner(), NewEphemeralSigner()
	now := time.Now()
	tok, _, _ := s1.Mint(uuid.New(), uuid.New(), []string{"echo"}, now)
	if _, err := s2.Verifier().Verify(tok, now); err == nil {
		t.Fatal("expected verification with a different key to fail")
	}
}

func TestTamperRejected(t *testing.T) {
	s := NewEphemeralSigner()
	now := time.Now()
	tok, _, _ := s.Mint(uuid.New(), uuid.New(), []string{"echo"}, now)

	r := []rune(tok)
	const i = 20 // inside the base64 body, past the "v4.public." header
	if r[i] == 'A' {
		r[i] = 'B'
	} else {
		r[i] = 'A'
	}
	if _, err := s.Verifier().Verify(string(r), now); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestSeedHexRoundTrip(t *testing.T) {
	src := NewEphemeralSigner()
	seed := src.secret.ExportSeedHex()
	pub := src.PublicKeyHex()

	signer, err := NewSignerFromSeedHex(seed)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifierFromHex(pub)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	tok, _, err := signer.Mint(uuid.New(), uuid.New(), []string{"echo"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(tok, now); err != nil {
		t.Fatalf("cross-instance verify with hex keys failed: %v", err)
	}
}
