# mock-oidc

A tiny, standards-compliant **OIDC / OAuth2 provider** used as the zero-dependency default identity provider for `secure-agent-platform`. It is a drop-in alternative to Keycloak, so `docker compose up` gives you real OIDC login with no external service.

> **Dev only.** There is no credential store — you *pick* a user rather than authenticate. All state is in memory and a fresh RSA signing key is generated on every boot. Never use this in production. The production/staging path uses real Keycloak; nothing in the platform branches on which provider issued a token — everything keys off the claims below.

## Flow

Authorization Code + **PKCE (S256 required)** with real **RS256** signing.

```
browser ──/authorize──▶ mock-oidc         pick Alice/Bob (or login=<sub> for curl)
        ◀─302 code──────                   redirect_uri?code=…&state=…
SPA     ──/token──────▶ mock-oidc         code + code_verifier  ──▶ access + id + refresh
        ◀─JWTs──────────
```

## Endpoints

| Method     | Path                                | Purpose                                          |
| ---------- | ----------------------------------- | ------------------------------------------------ |
| `GET`      | `/.well-known/openid-configuration` | Discovery document (all URLs derived from issuer)|
| `GET`      | `/jwks`                             | JWK Set — one RSA key `{kty,use,alg,kid,n,e}`    |
| `GET`      | `/authorize`                        | Login picker + code issuance (PKCE S256)         |
| `POST`     | `/token`                            | `authorization_code` and `refresh_token` grants  |
| `GET`      | `/userinfo`                         | Claims for a valid Bearer access token           |
| `GET/POST` | `/logout`                           | `end_session_endpoint` (best-effort)             |
| `POST`     | `/revoke`                           | Token revocation (best-effort, RFC 7009)         |

`/authorize` accepts `login=<subject>` (e.g. `login=alice`) to skip the HTML picker and issue a code immediately, so the whole flow can be driven headlessly from `curl` or a test.

## Token claims

Both tokens are RS256 JWTs carrying `iss`, `sub`, `iat`, `exp` plus the custom claims the control plane authorizes off — `tenant_id` (seeded tenant UUID), `email`, `name`, `roles`. The audiences differ:

| Token          | `aud`                              | TTL    | Extra          |
| -------------- | ---------------------------------- | ------ | -------------- |
| `access_token` | `sap-control-plane` (API audience) | ~15m   | —              |
| `id_token`     | `sap-dashboard` (SPA client)       | ~15m   | `nonce` if sent|

`tenant_id` is the linchpin of tenant isolation: the control plane projects it into the Postgres `app.current_tenant` GUC that row-level security filters on. The demo identities live in `control-plane/internal/devseed`.

Refresh tokens are opaque and **rotated** on every use (the presented token is invalidated and a new one issued).

## Configuration

| Env var                  | Default                 | Meaning                                  |
| ------------------------ | ----------------------- | ---------------------------------------- |
| `MOCK_OIDC_ADDR`         | `:9000`                 | Listen address                           |
| `MOCK_OIDC_ISSUER`       | `http://localhost:9000` | External issuer URL (`iss` + endpoint base) |
| `MOCK_OIDC_CLIENT_ID`    | `sap-dashboard`         | Public SPA client / ID-token audience    |
| `MOCK_OIDC_API_AUDIENCE` | `sap-control-plane`     | Access-token audience                    |

In compose the browser reaches the issuer at `http://localhost:9000` while the control plane reaches it at `http://mock-oidc:9000`; set `MOCK_OIDC_ISSUER` to a value both sides agree on, since it is what appears as `iss` and as the base of every URL in the discovery document.

## Run & test

```bash
export GOTOOLCHAIN=auto
go run  ./cmd/mock-oidc/      # starts on :9000
go test ./cmd/mock-oidc/      # full auth-code+PKCE flow, verified with go-oidc
```

The test exercises the complete flow in-process and then verifies the returned tokens with `github.com/coreos/go-oidc/v3/oidc` (discovery, JWKS-based RS256 signature check, and audience enforcement), proving real go-oidc compatibility.
