# Observability stack

`app → OTLP → otel-collector → Jaeger (traces) + Prometheus (metrics) → Grafana`

Enabled by the compose `observability` profile. The control plane and worker export OTLP **only** when `OTEL_EXPORTER_OTLP_ENDPOINT` is set — an empty endpoint is a no-op, so the default profile stays lean.

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317 \
  docker compose -f ../compose/docker-compose.yaml --profile observability up -d --build
```

| File                                              | Role                                                          |
| ------------------------------------------------- | ------------------------------------------------------------- |
| `otel-collector-config.yaml`                      | OTLP gRPC :4317 / HTTP :4318 in; traces→Jaeger, metrics→:8889. |
| `prometheus/prometheus.yml`                       | Scrapes the collector's `:8889` (app metrics) + `:8888`.      |
| `grafana/provisioning/datasources/…`              | Prometheus + Jaeger datasources (auto-loaded).                |
| `grafana/provisioning/dashboards/…`               | Dashboard provider.                                           |
| `grafana/dashboards/sap-platform.json`            | The **Overview** dashboard.                                   |

## Dashboard panels ↔ platform metrics

The instrumentation lands separately; the dashboard is built against the metric names the code will emit:

| Panel                          | PromQL                                             |
| ------------------------------ | -------------------------------------------------- |
| Total / rate of agent runs     | `sum(sap_runs_total)` · `sum(rate(sap_runs_total[5m]))` |
| Tool calls by outcome (allow/deny) | `sum by (outcome) (rate(sap_tool_calls_total[5m]))` |
| Tool calls by tool             | `sum by (tool) (rate(sap_tool_calls_total[5m]))`   |
| Guardrail blocks by reason     | `sum by (reason) (rate(sap_guard_blocks_total[5m]))` |
| Credential denials by reason   | `sum by (reason) (rate(sap_credential_denied_total[5m]))` |
| RLS denials (tenant isolation) | `sum(sap_rls_denied_total)` (turns red if > 0)     |

## Validation performed

- YAML parse for every config file (`python3 -c "import yaml; ..."`).
- `docker compose --profile observability config --quiet`.
- (RAM permitting) start `otel-collector` alone to confirm the config loads, then stop it.

**RAM:** ~1 GB total. Do **not** run alongside the `keycloak` profile on a ~2 GB host.
