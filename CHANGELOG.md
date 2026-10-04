# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Monorepo scaffolding**: build system (`Makefile`), gRPC contract (`buf`), and CI / CodeQL / security-gate / diagram workflows.
- **Multi-tenant isolation** via PostgreSQL row-level security (`FORCE`, `USING` + `WITH CHECK`, a `NOBYPASSRLS` runtime role, a single `WithTenant` choke point), proven by a testcontainers integration suite.
- **Control plane** (Go): REST + SSE API, an internal gRPC tool proxy, OIDC auth (JWKS validation, provider-agnostic), a PASETO scoped-credential minter + revocation store, an SSRF / path-traversal policy guard, and an append-only audit log. Plus a bundled, `go-oidc`-verified mock OIDC provider.
- **Agent worker** (Python): a custom plan→act→observe core, a deterministic mock LLM, and a tool-proxy client that holds only a scoped credential.
- **React dashboard**: tenants, live agent runs (header-authenticated SSE), and the audit log; OIDC authorization-code + PKCE.
- **Observability**: OpenTelemetry traces + metrics across the platform.
- **Deploy**: docker-compose profiles (default / Keycloak / observability), a Helm chart, kustomize, and Terraform.
- **Docs**: security & threat models, ADRs, and eight Mermaid diagrams.
