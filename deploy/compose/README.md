# Compose profiles

The stack ships three profiles. **Keep the default tiny** and never run `keycloak` + `observability` together on a ~2 GB host.

| Profile         | Adds                                             | Extra RAM | Host ports |
| --------------- | ------------------------------------------------ | --------- | ---------- |
| _default_       | postgres, mock-oidc, control-plane, agent-worker, frontend | baseline | 5432, 9000, 8080/9090, 5173 |
| `keycloak`      | keycloak (real IdP)                              | ~700 MB   | 8081 |
| `observability` | otel-collector, jaeger, prometheus, grafana      | ~1 GB     | 4317, 16686, 9091, 3000 |

```bash
# default — zero secrets, mock IdP + mock LLM
docker compose -f docker-compose.yaml up -d --build

# stop everything (any profile) and drop volumes
docker compose -f docker-compose.yaml --profile keycloak --profile observability down -v
```

## `keycloak` profile

The base file defines the `keycloak` service; the **override** file rewires the control plane and rebuilds the SPA against the Keycloak issuer in one command:

```bash
docker compose -f docker-compose.yaml -f docker-compose.keycloak.yaml \
  --profile keycloak up -d --build
```

Keycloak imports `deploy/keycloak/realm-export.json` on startup (~40 s). Log in at the dashboard as **alice/alice** (Tenant A admin) or **bob/bob** (Tenant B). Admin console: <http://localhost:8081> (admin/admin, dev only).

If you prefer to wire it by hand instead of the override, the only three knobs the control plane needs are:

```bash
OIDC_ISSUER_URL=http://localhost:8081/realms/secure-agent   # token `iss` / browser-facing
OIDC_DISCOVERY_URL=http://keycloak:8080/realms/secure-agent  # backend discovery/JWKS (Docker-internal)
OIDC_AUDIENCE=sap-control-plane
```

Why the two different URLs: the browser reaches Keycloak at `localhost:8081`, so tokens carry `iss=http://localhost:8081/...`; the control plane reaches it over the Docker network at `keycloak:8080`. Keycloak pins the issuer to the front-end URL (`KC_HOSTNAME`) while resolving discovery/JWKS URLs from the request host (`KC_HOSTNAME_BACKCHANNEL_DYNAMIC=true`) — exactly the split the verifier expects. See `deploy/keycloak/README.md` for how the realm produces the `tenant_id`, audience and `realm_access.roles` claims.

## `observability` profile

The app emits OTLP only when an endpoint is set (empty = no-op), so enable it by exporting the endpoint and starting the profile:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317 \
  docker compose -f docker-compose.yaml --profile observability up -d --build
```

- Grafana  <http://localhost:3000>  (anonymous viewer; admin/admin) — datasources and the **Secure Agent Platform — Overview** dashboard are auto-provisioned.
- Jaeger   <http://localhost:16686>
- Prometheus <http://localhost:9091> (host 9091 avoids the control plane's :9090)

Path: app → OTLP → otel-collector → Jaeger (traces) + Prometheus scrape of the collector's `:8889` (metrics) → Grafana.

## Validation

```bash
docker compose -f docker-compose.yaml config --quiet                       # default
docker compose -f docker-compose.yaml --profile keycloak config --quiet
docker compose -f docker-compose.yaml --profile observability config --quiet
docker compose -f docker-compose.yaml -f docker-compose.keycloak.yaml \
  --profile keycloak config --quiet                                        # override
```
