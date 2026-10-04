# Threat model

A lightweight, STRIDE-flavored threat model for `secure-agent-platform`. It complements [`security-model.md`](security-model.md), which describes the controls; this document enumerates the threats those controls answer, and the residual risk.

## Assets

| Asset | Why it matters |
|---|---|
| Tenant data (runs, events, audit) | Confidentiality/integrity across tenants is the core promise. |
| The scoped-credential signing key | Forging it would let any run call any tool for any tenant. |
| The audit log | Its integrity is what makes the platform accountable. |
| End-user sessions / tokens | Impersonation would breach a tenant. |

## Actors

- **Legitimate tenant user** — authenticated, scoped to one tenant.
- **Malicious/curious tenant** — a valid user of tenant B trying to reach tenant A's data.
- **A manipulated agent** — the worker steered by a prompt-injection payload into misusing tools or exfiltrating data. Assumed to be *possible*, and contained by design.
- **Network attacker** — on the local network (out of scope for the dev profile; see limitations).

## Threats and mitigations

| # | Threat (STRIDE) | Vector | Mitigation |
|---|---|---|---|
| T1 | **Information disclosure** — cross-tenant read | Tenant B calls the API with tenant A's `run_id` (IDOR) | RLS filters every query to `app.current_tenant`; the API returns 404, not 403, so existence isn't leaked. Proven by the isolation test. |
| T2 | **Tampering** — cross-tenant write | Insert/update a row with another tenant's `tenant_id` | `WITH CHECK` rejects the insert; `USING` makes the update match 0 rows. |
| T3 | **Elevation of privilege** — RLS bypass | Use of a superuser / table-owner / `BYPASSRLS` connection at runtime | Runtime role is non-super, non-owner, `NOBYPASSRLS`, and `FORCE RLS` is on. A test asserts these invariants. |
| T4 | **Spoofing** — forged identity | Fake or altered JWT | Signature validated against the IdP's JWKS; `iss`/`aud`/`exp` checked; `tenant_id` comes only from the verified token. |
| T5 | **Spoofing** — forged credential | Worker mints/alters its own tool credential | PASETO v4.public: the worker has no signing key and cannot forge; any tamper fails signature verification (tested). |
| T6 | **Elevation** — out-of-scope tool use | Manipulated agent calls a tool it wasn't granted | Tool proxy checks the credential's per-run allow-list **and** the global registry before executing; denials are audited. |
| T7 | **Elevation** — credential replay after run | Reuse of a leaked token after the run ends | ~10-minute expiry + revocation of the run's credentials at completion, checked on every call. |
| T8 | **Tampering** — SSRF / path traversal via tool args | Agent passes `http://169.254.169.254/…` or `../../etc/passwd` | Policy guard blocks private/link-local/CGNAT/metadata IPs (with DNS-rebinding pinning) server-side; the path-confinement helper is tested and wired when a filesystem tool ships. |
| T9 | **Prompt injection** — instruction smuggling | Malicious text in the objective or a tool result | Tool results treated as data; heuristic corpus checks; blast radius bounded by T5–T8. Not claimed to be fully prevented. |
| T10 | **Repudiation** | Denying an action happened | Audit log — append-only at the DB level (runtime role has no `UPDATE`/`DELETE`) — records login, run, credential mint/revoke, and every tool allow/deny/execute with a result fingerprint. |

## Residual risks (accepted for this project)

- Prompt injection is **contained, not eliminated** (T9). A determined payload within the granted tool scope can still do anything those tools permit — which is why the default tools are read-only and side-effect-free.
- Dev-grade secrets, plaintext local transport, no rate limiting, and single-instance SSE — all listed in `security-model.md` → *Honest limitations*.
- The mock IdP is not a hardened credential store; use Keycloak or a real IdP for anything beyond a demo.
