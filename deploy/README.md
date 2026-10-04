# deploy/

Deployment & infrastructure-as-code for the Secure Agent Platform. Four ways to run it, in increasing distance from the laptop:

| Path                    | What it is                                              | Primary use                    |
| ----------------------- | ------------------------------------------------------- | ------------------------------ |
| [`compose/`](compose/)  | Docker Compose stack with three profiles                | Local dev & the demo           |
| [`helm/`](helm/secure-agent-platform/) | Umbrella Helm chart (the **primary** k8s path)          | Any Kubernetes cluster         |
| [`k8s/`](k8s/)          | kustomize base + `dev` overlay                          | Lightweight k8s / GitOps demo  |
| [`terraform/`](terraform/) | kind+Helm (applyable) · AWS VPC/EKS/RDS (reference)     | Provisioning                   |

Supporting configs: [`keycloak/`](keycloak/) (realm export) and [`observability/`](observability/) (OTel/Jaeger/Prometheus/Grafana) feed the compose profiles and are copied into the Helm chart.

## RAM note (important)

The host has ~2 GB free. **Never run the `keycloak` and `observability` compose profiles at the same time.** Rough extra cost over the default stack:

| Profile / stack        | Extra RAM |
| ---------------------- | --------- |
| default (compose)      | baseline  |
| `keycloak`             | ~700 MB   |
| `observability`        | ~1 GB     |
| kind + Helm (terraform local) | heavy (image pulls) |

Prefer the static validators below; only start heavy containers briefly and tear them straight down.

## Run each path

```bash
# 1) Compose — default (zero secrets)
docker compose -f deploy/compose/docker-compose.yaml up -d --build
#    + Keycloak (one command via the override)
docker compose -f deploy/compose/docker-compose.yaml \
  -f deploy/compose/docker-compose.keycloak.yaml --profile keycloak up -d --build
#    + Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317 \
  docker compose -f deploy/compose/docker-compose.yaml --profile observability up -d --build
docker compose -f deploy/compose/docker-compose.yaml \
  --profile keycloak --profile observability down -v      # tear down everything

# 2) Helm
helm install sap deploy/helm/secure-agent-platform -n sap --create-namespace

# 3) kustomize
kubectl apply -k deploy/k8s/overlays/dev

# 4) Terraform (local kind + Helm)
cd deploy/terraform/local && terraform init && terraform apply
```

## Validate everything (no cluster required)

```bash
# Compose config resolves on every profile
docker compose -f deploy/compose/docker-compose.yaml config --quiet
docker compose -f deploy/compose/docker-compose.yaml --profile keycloak config --quiet
docker compose -f deploy/compose/docker-compose.yaml --profile observability config --quiet

# Keycloak realm + Grafana dashboard are well-formed JSON
python3 -m json.tool deploy/keycloak/realm-export.json > /dev/null

# Helm
helm lint deploy/helm/secure-agent-platform
helm template deploy/helm/secure-agent-platform | kubeconform -strict -summary

# kustomize
kubectl kustomize deploy/k8s/overlays/dev | kubeconform -strict -summary

# Terraform
cd deploy/terraform/local        && terraform init && terraform validate && terraform fmt -check
cd deploy/terraform/modules/aws  && terraform init -backend=false && terraform validate && terraform fmt -check && tflint
```

See each subdirectory's README for details. Everything here is **validated, not production-hardened** — the honest scope is stated in the chart README.
