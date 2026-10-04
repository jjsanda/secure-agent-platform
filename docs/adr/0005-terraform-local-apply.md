# ADR-0005: Terraform that applies to a local kind cluster

- **Status:** accepted
- **Date:** 2026-07-01

## Context

For a portfolio project, cloud modules that only ever run `terraform validate` prove little — they can be plausible-looking and still not work. A full cloud deployment, on the other hand, needs a real account and money.

## Decision

Lead with a **genuinely applyable local stack**: `deploy/terraform/local` uses the `kind`, `helm`, and `kubernetes` providers to stand up a local Kubernetes cluster and install the platform's Helm chart — `terraform apply` really runs, with no cloud credentials. Keep **`deploy/terraform/modules/aws`** (VPC + EKS + RDS) as clearly-labeled *reference* modules that stay `fmt`/`validate`/`tflint`-clean in CI but are not applied.

## Consequences

- The IaC is provably real end-to-end via the local path, while the AWS modules remain a validated reference for the cloud equivalent.
- Anyone can `terraform apply` on their own machine and get a running cluster.
- The AWS modules carry an explicit "reference, not applied" caveat so nobody mistakes them for a tested production deployment.

## Alternatives considered

- **Validate-only cloud modules** — rejected: not credible on their own.
- **A real cloud deployment** — rejected: requires accounts/credentials and ongoing cost, defeating the "clone and run" goal.
