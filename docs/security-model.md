# Security model

This document describes what `secure-agent-platform` protects, how, and — just as importantly — **what it does not protect against**. The whole project is a demonstration of secure design, so the limitations section is not an afterthought; it is the point.

## Goals

1. **Tenant isolation.** One tenant's data and agent runs are invisible and untouchable to another, enforced by the database, not just the application.
2. **Least privilege for agents.** An agent run receives the minimum capability it needs — a short-lived credential scoped to one run and an explicit set of tools — and nothing more.
3. **Mediated, audited side effects.** Every action with an external effect goes through one choke point that validates and records it.
4. **Standard, verifiable auth.** Identity is established by OIDC/OAuth2 with PKCE and JWKS-validated tokens — no bespoke auth.

## Trust boundaries

```
Browser ──[TLS/JWT]──> Control Plane ──[gRPC]──> Agent Worker
   │                        │                         │
   └─[OIDC/PKCE]─> IdP      ├─[RLS tx]─> PostgreSQL    └─ holds ONLY a scoped credential
                            └─[tool proxy]─> tools (allow-listed, guarded)
```

- The **browser** is untrusted; it only ever holds tokens the IdP issued and calls the API with them.
- The **agent worker** is *semi-trusted*: it runs model-directed logic and is assumed to be steerable by a malicious prompt. It therefore holds **no** database access and **no** long-lived secret — only an opaque, short-lived, run-scoped credential. Everything it can do flows through the tool proxy.
- The **control plane** is the trusted core: it mints credentials, validates them, enforces RLS, and owns the audit log.

## Identities and roles

- **End users** authenticate via OIDC (Keycloak or the bundled mock). The access token carries `sub`, a custom `tenant_id`, `email`, `name`, and roles. The control plane validates the signature against the provider's JWKS and the `aud`/`iss`/`exp` claims before trusting anything.
- **Database roles** are three-tier, because superusers and table owners bypass row-level security:
  - `sap_owner` owns the schema and runs migrations (DDL only). Not a superuser.
  - `sap_app` is the runtime role. DML only, **not** a table owner, **`NOBYPASSRLS`**. All request traffic uses this role, so RLS always applies.
  - `sap_migrator` has `BYPASSRLS` and is used *only* by seeding/admin tooling, never by the request path.

## Tenant isolation (row-level security)

Every tenant table has a `tenant_id` column, `ENABLE` **and `FORCE ROW LEVEL SECURITY`**, and a policy:

```sql
USING      (tenant_id = NULLIF(current_setting('app.current_tenant', true), '')::uuid)
WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant', true), '')::uuid)
```

Each request runs inside `WithTenant`, which opens a transaction and sets `app.current_tenant` with a **parameterized** `set_config(..., local => true)` — no SQL string building (no injection surface) and transaction-scoped (no leakage across pooled connections). `NULLIF(..., '')` makes the predicate **fail-closed**: an unset context — or a pooled connection whose local setting has reset to an empty string — yields `NULL`, so no rows match and inserts are rejected. `FORCE` closes the table-owner bypass; `NOBYPASSRLS` closes the role bypass. See [ADR-0002](adr/0002-postgres-rls-for-tenant-isolation.md).

This is verified, not asserted: `control-plane/test` spins up a real PostgreSQL and proves tenant A cannot read, insert-for, or update tenant B, that no-context denies everything, and that the app role has no bypass.

## Scoped credentials

On run start the control plane mints a **PASETO v4.public** (Ed25519) token scoped to `{run_id, tenant_id, allowed_tools, iat, nbf, exp≈10m, jti}`, persists the `jti` in a tenant-scoped revocation table, and hands the token to the worker. PASETO is chosen over JWT because it has no in-band algorithm field (the `alg:none` and HS/RS-confusion attacks are structurally impossible) and cleanly separates the signing key (minter) from the verification key (proxy). See [ADR-0003](adr/0003-paseto-scoped-credentials.md).

The credential is **short-lived, single-run, and revoked at run end**, so a leaked token has a small and bounded blast radius.

## The tool proxy and policy guard

The worker performs side effects only by calling `ToolProxyService.ExecuteTool`, presenting the credential as gRPC metadata. A server interceptor validates it *before* the handler runs, in order: signature → expiry/nbf → revocation store (by `jti`, tenant-scoped) → then the handler checks run binding → the credential's tool allow-list → the global tool registry → the policy guard (argument schema and SSRF / private-IP blocking; path-traversal confinement is a tested helper, wired when a filesystem tool ships). Only then does the tool execute. Every allow, deny, and execution is written to the audit log — which the runtime role may INSERT into but cannot UPDATE or DELETE (`REVOKE`d), so the trail is append-only at the database level — with a fingerprint of the result.

## Prompt-injection posture

The worker treats tool results and retrieved content strictly as **data**, never as instructions, and runs a heuristic guard over inputs and tool outputs. A curated corpus of injection attempts (`agent-worker/.../security/corpus`) is exercised by tests. But the real protection is **architectural**, not detection-based:

- the tool surface is **allow-listed and read-only by default** — there is no destructive tool;
- the agent's capability is a **short-lived, per-run, tool-scoped credential**;
- irreversible actions would require human approval;
- everything is audited.

## Honest limitations

- **This is not "injection-proof."** Heuristic detection is best-effort and can be bypassed; the design assumes the agent *can* be manipulated and contains the blast radius instead of promising it can't be.
- **Dev-grade secrets.** The default profile uses fixed dev passwords and an ephemeral signing key so it runs with zero setup. A real deployment would use a secrets manager and a stable, rotated key.
- **The mock IdP is dev-only.** It is standards-compliant enough to be validated by `go-oidc`, but it is not a hardened identity provider — use the Keycloak profile (or your own IdP) for anything real.
- **Single-instance SSE.** The live event fan-out is in-memory; a horizontally-scaled deployment would back it with Postgres `LISTEN/NOTIFY` or a broker. Events are always durably persisted regardless.
- **Transport.** Local traffic is plaintext HTTP/gRPC; production would terminate TLS and use mTLS between control plane and worker.
- **No rate limiting / quotas** on run creation in this demo.

These are deliberate scoping choices for a portfolio project, called out so the security claims stay truthful.
