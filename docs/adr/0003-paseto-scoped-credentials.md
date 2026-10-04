# ADR-0003: PASETO for scoped tool credentials

- **Status:** accepted
- **Date:** 2026-07-01

## Context

An agent run must carry a capability that says "this run, for this tenant, may call these tools, until this time." The worker is assumed to be manipulable by prompt injection, so the credential must be unforgeable by the party that holds it, and it must be short-lived and revocable. The obvious default is a JWT.

## Decision

Use **PASETO v4.public** (Ed25519) tokens, minted by the control plane and verified by the tool proxy.

- **Asymmetric** signing means the signing key (minter) is separable from the verification key (proxy); the worker holds neither — only the opaque token.
- PASETO has **no in-band algorithm field**, so the entire `alg:none` / HS-vs-RS key-confusion class of JWT vulnerabilities is structurally impossible.
- Claims: `run_id`, `tenant_id`, `allowed_tools`, `iat`, `nbf`, `exp` (~10 min), `jti`. The `jti` is persisted in a tenant-scoped revocation table and checked on every tool call; the run's credentials are revoked at completion.

## Consequences

- A leaked credential is bounded: one run, one tenant, a fixed tool set, ~10 minutes, revocable.
- Slightly less ecosystem familiarity than JWT; mitigated by the token being an internal detail never exposed to clients.

## Alternatives considered

- **JWT (EdDSA)** — viable and more familiar, but it reintroduces the algorithm-agility footguns unless the verifier strictly allow-lists the algorithm. Recorded here as the documented fallback.
- **JWT (HS256)** — rejected: a shared secret between minter and verifier, and the worst of the algorithm-confusion history.
- **Opaque tokens + DB lookup only** — rejected as the sole mechanism: pushes all validation to a DB round trip and loses the self-describing, offline-verifiable scope. (A revocation store is still used, as a second layer.)
