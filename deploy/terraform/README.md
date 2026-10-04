# Terraform

Two independent stacks with **deliberately different intents**, recorded here honestly so the scope is unambiguous.

## `local/` — genuinely applyable

Stands up a local [kind](https://kind.sigs.k8s.io/) cluster (`tehcyx/kind` provider) and installs the umbrella Helm chart onto it (`helm` provider). This is real: `terraform apply` works end-to-end.

```bash
cd deploy/terraform/local
terraform init
terraform apply                     # creates the kind cluster + installs the chart
# ... use the stack ...
terraform destroy
```

Toggles (`-var`): `keycloak_enabled`, `observability_enabled`, `postgres_persistence`, `node_image`, `cluster_name`, `namespace`.

A full `apply` is **heavy** (pulls `kindest/node` + every service image), so CI runs only the static checks below — not `apply`.

## `modules/aws/` — reference, NOT applied

A reference AWS topology — **VPC** (public/private subnets, NAT), **EKS** (cluster + managed node group in private subnets) and **RDS Postgres** (private, encrypted, multi-AZ) — written with raw resources so the wiring is legible. It is a **module** (no provider block, no backend); a real root would call it, pass a configured `aws` provider + region, and supply `db_password` from a secrets manager. It is intentionally never applied here — it is a design artifact showing how the same app maps onto managed infrastructure (RDS replaces the in-chart Postgres via the chart's `externalDatabase.enabled`).

```hcl
# example root (not included)
provider "aws" { region = "eu-central-1" }

module "platform" {
  source      = "../modules/aws"
  name        = "sap-prod"
  db_password = var.db_password # from TF_VAR_db_password / Secrets Manager
}
```

## Validation performed

| Command                                   | local | modules/aws |
| ----------------------------------------- | :---: | :---------: |
| `terraform init`                          |  ok   |     ok      |
| `terraform validate`                      |  ok   |     ok      |
| `terraform fmt -check`                    |  ok   |     ok      |
| `tflint`                                  |   —   |     ok      |

```bash
cd deploy/terraform/local     && terraform init && terraform validate && terraform fmt -check
cd deploy/terraform/modules/aws && terraform init -backend=false && terraform validate && terraform fmt -check && tflint
```

> `.terraform/` provider caches and `*.tfstate` are git-ignored. Never commit `terraform.tfvars` containing `db_password`.
