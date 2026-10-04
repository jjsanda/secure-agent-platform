# secure-agent-platform Helm chart

Umbrella chart that renders the whole platform — Postgres (with the three-tier RLS role bootstrap), control-plane, agent-worker, the identity provider (bundled mock **or** Keycloak) and the frontend — as one release.

> **Scope: validated, not production-hardened.** It passes `helm lint` and `kubeconform -strict`, wires the exact env-var contract from the compose stack, and includes baseline operational safeguards (non-root/seccomp/dropped-caps security contexts, credentials-in-Secret, NetworkPolicies, HPAs, probes). It is **not** audited for production: the frontend runs stock nginx (not non-root), passwords default to dev values, and full tracing/metrics backends are the compose profile's job.

## Install

```bash
# default: in-chart Postgres + mock OIDC + mock LLM
helm install sap deploy/helm/secure-agent-platform -n sap --create-namespace

# real Keycloak instead of the mock IdP
helm install sap deploy/helm/secure-agent-platform -n sap --create-namespace \
  --set keycloak.enabled=true

# emit OTLP to an in-cluster collector, and autoscale the hot paths
helm install sap deploy/helm/secure-agent-platform -n sap --create-namespace \
  --set observability.enabled=true \
  --set controlPlane.autoscaling.enabled=true \
  --set agentWorker.autoscaling.enabled=true
```

## Key toggles (`values.yaml`)

| Value                                   | Default | Effect                                                        |
| --------------------------------------- | ------- | ------------------------------------------------------------- |
| `keycloak.enabled`                      | `false` | Deploy Keycloak + import the realm; disable mock-oidc; repoint the control plane. |
| `observability.enabled`                 | `false` | Deploy an OTel collector and set `OTEL_EXPORTER_OTLP_ENDPOINT`. |
| `controlPlane.autoscaling.enabled`      | `false` | HPA (CPU) for the control plane; drops the static `replicas`. |
| `agentWorker.autoscaling.enabled`       | `false` | HPA (CPU) for the worker.                                     |
| `ingress.enabled`                       | `false` | Ingress: `/` → frontend, `/api` → control-plane.              |
| `networkPolicy.enabled`                 | `true`  | Default-deny ingress + intra-app + web allows.                |
| `externalDatabase.enabled`              | `false` | Use a managed Postgres (e.g. RDS) instead of the in-chart one. |
| `postgres.persistence.enabled`          | `true`  | PVC-backed data dir (else `emptyDir`).                        |
| `*.image.repository` / `*.image.tag`    | —       | Per-component image repo/tag (tag defaults to `appVersion`).  |
| `*.resources`                           | —       | Requests/limits per component.                                |

## How identity is wired

The control plane is provider-agnostic; the chart only flips three env vars:

- `OIDC_ISSUER_URL` — browser-facing issuer (token `iss`). Mock: `mockOidc.issuer`; Keycloak: `keycloak.issuerUrl`.
- `OIDC_DISCOVERY_URL` — in-cluster Service where the backend fetches discovery/JWKS (`…-mock-oidc:9000` or `…-keycloak:8080/realms/secure-agent`).
- `OIDC_AUDIENCE` — always `sap-control-plane`.

Keycloak pins the front-end URL (`KC_HOSTNAME`, derived from `keycloak.issuerUrl`) and resolves backchannel URLs from the request host (`KC_HOSTNAME_BACKCHANNEL_DYNAMIC=true`), so the browser/backend URL split matches the verifier. **In a real cluster set `keycloak.issuerUrl` (and `ingress.host`) to your externally-reachable URL** — `localhost:8081` only works with a port-forward.

The realm JSON and the Postgres role-bootstrap script live in `files/` (copied from `deploy/keycloak/` and `deploy/compose/initdb/`) so the chart is self-contained.

## Secrets

`templates/secret.yaml` holds the four Postgres role passwords **and** the fully formed connection URLs. The control plane consumes `DATABASE_URL` / `MIGRATE_DATABASE_URL` / `MIGRATOR_DATABASE_URL` via `secretKeyRef`, so no password appears in a container spec. A pod-rolling `checksum/secret` annotation is set.

## Validation

```bash
helm lint deploy/helm/secure-agent-platform
helm template deploy/helm/secure-agent-platform | kubeconform -strict -summary
# exercise the toggles:
helm template deploy/helm/secure-agent-platform \
  --set keycloak.enabled=true --set observability.enabled=true \
  --set controlPlane.autoscaling.enabled=true --set ingress.enabled=true \
  | kubeconform -strict -summary
```

Both were run for this chart: **lint 0 failed**, kubeconform **16/16** (default) and **23/23** (all toggles) valid.
