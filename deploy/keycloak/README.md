# Keycloak realm — `secure-agent`

`realm-export.json` is imported on startup (`start-dev --import-realm`). It makes Keycloak a **drop-in** replacement for the bundled mock OIDC provider: no control-plane code changes, only the three `OIDC_*` env vars (see `../compose/README.md`).

## How the required claims are produced

The control-plane verifier (`control-plane/internal/auth`) needs an **access token** with audience `sap-control-plane` carrying `sub`, `tenant_id`, `email`, `name`, and roles (top-level `roles` **or** `realm_access.roles`). The realm delivers each of these:

| Claim               | Source in the realm                                                                 |
| ------------------- | ----------------------------------------------------------------------------------- |
| `aud = sap-control-plane` | **Audience mapper** on the `sap-dashboard` client (`oidc-audience-mapper`, `included.custom.audience`). |
| `tenant_id` (String)| **User-attribute mapper** (`oidc-usermodel-attribute-mapper`) projecting the user's `tenant_id` attribute, `access.token.claim=true`, `jsonType.label=String`. |
| `realm_access.roles`| Keycloak's built-in **realm roles** mapper (default `roles` client scope). Users get the roles by direct assignment (`realmRoles`). |
| `email`             | Default `email` client scope.                                                       |
| `name`              | Default `profile` client scope (full-name mapper) — `firstName`+`lastName`.         |
| `iss`               | `KC_HOSTNAME` front-end URL → `http://localhost:8081/realms/secure-agent`.           |

The `tenant_id` attribute holds the **fixed** tenant UUIDs the control plane seeds and enforces via Postgres RLS:

| User  | Password | `tenant_id`                            | Realm roles              |
| ----- | -------- | -------------------------------------- | ------------------------ |
| alice | alice    | `11111111-1111-1111-1111-111111111111` | `tenant-admin`, `member` |
| bob   | bob      | `22222222-2222-2222-2222-222222222222` | `member`                 |

(Passwords equal the usernames — **dev demo only**.) A `platform-admin` realm role also exists (reserved; no demo user carries it).

> Names are plain (`Alice Tenant A Admin`, not `Alice (Tenant A admin)`): the Keycloak 26 declarative user profile validates `firstName`/`lastName` with the person-name validator, and punctuation like `()` fails it — which otherwise blocks login with "Account is not fully set up". The custom `tenant_id` attribute is fine as an unmanaged attribute (verified).

## The `sap-dashboard` client

Public SPA client, **authorization code flow with PKCE S256 required**, no secret. Redirect URIs `http://localhost:5173/*`, web origins `http://localhost:5173`. Direct-access grants are enabled as a **dev convenience** so you can mint a token from the CLI to inspect the claims:

```bash
# only works while the keycloak profile is running
TOKEN=$(curl -s http://localhost:8081/realms/secure-agent/protocol/openid-connect/token \
  -d grant_type=password -d client_id=sap-dashboard \
  -d username=alice -d password=alice -d scope=openid | jq -r .access_token)
# decode the payload — expect aud=sap-control-plane, tenant_id=1111...,
# realm_access.roles=[tenant-admin, member]
echo "$TOKEN" | cut -d. -f2 | tr '_-' '/+' | base64 -d 2>/dev/null | jq .
```

## Validation performed

- `python3 -m json.tool realm-export.json` — well-formed.
- `docker compose --profile keycloak config --quiet` — service resolves.
- **End-to-end (actually run):** started Keycloak 26.1 alone importing this exact file, then confirmed via `curl`:
  - discovery `issuer = http://localhost:8081/realms/secure-agent`;
  - `alice`/`bob` log in via direct grant (no "Account is not fully set up");
  - decoded access tokens carry `aud=sap-control-plane`, `tenant_id=1111…/2222…`, `realm_access.roles=[tenant-admin,member]/[member]`, plus `email`/`name`/`iss` — i.e. the full control-plane contract. Torn down after.

> The realm importer is **strict** and rejected an earlier `__doc` comment key, so the JSON carries no inline comments — this file is documented here instead.
