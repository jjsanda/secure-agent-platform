#!/usr/bin/env bash
# Generate a dev PASETO v4.public (Ed25519) keypair, hex-encoded.
# The control plane signs scoped credentials with the private key; the tool proxy
# verifies with the public key. DEV ONLY — never commit real keys.
set -euo pipefail

command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
openssl genpkey -algorithm ed25519 -out "$tmp/ed25519.pem" 2>/dev/null

to_hex() { od -An -v -tx1 | tr -d ' \n'; }
# The raw 32-byte Ed25519 seed is the trailing 32 bytes of the PKCS#8 DER;
# the raw 32-byte public key is the trailing 32 bytes of the SPKI DER.
priv_hex="$(openssl pkey -in "$tmp/ed25519.pem" -outform DER        | tail -c 32 | to_hex)"
pub_hex="$( openssl pkey -in "$tmp/ed25519.pem" -pubout -outform DER | tail -c 32 | to_hex)"

cat <<EOF
# --- PASETO v4.public dev keypair (Ed25519, hex-encoded) -----------------------
# Add to your .env (dev only). Regenerate any time; credentials are short-lived.
PASETO_PRIVATE_KEY=${priv_hex}
PASETO_PUBLIC_KEY=${pub_hex}
EOF
