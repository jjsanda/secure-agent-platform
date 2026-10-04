# Contributing

Thanks for looking at `secure-agent-platform`. This is primarily a personal portfolio project, but the codebase is meant to read like a real one — clear boundaries, tests that prove the security claims, and reproducible tooling.

## Prerequisites

- Go 1.25+, Python 3.12+ with [`uv`](https://docs.astral.sh/uv/), Node 22+, Docker + Compose.
- `make tools` installs the extra CLIs (`buf`, `sqlc`, `migrate`, `golangci-lint`).

## Workflow

```bash
make up      # start the default (zero-secret) stack
make demo    # end-to-end demo: seed tenants, run an agent, print the audit log
make test    # unit + integration tests (incl. the tenant-isolation suite)
make lint    # gofmt / go vet / ruff / black / mypy
make proto   # regenerate gRPC stubs after editing proto/
make diagrams# re-render the Mermaid diagrams
```

## Conventions

- **Contracts first.** The gRPC `.proto` files and the SQL migrations are the source of truth; generated code (`buf generate`, `sqlc generate`) is committed and drift-checked in CI.
- **Every DB access goes through `WithTenant`.** Row-level security is the isolation guarantee; the app never adds `tenant_id` filters by hand. An architecture test enforces this.
- **The worker holds only a scoped credential.** No DB access and no long-lived secrets in the agent worker — all side effects go through the tool proxy.
- Conventional-commit style messages (`feat:`, `fix:`, `docs:`, `test:` …).

## Before opening a PR

`make lint && make test` must pass. New security-relevant behavior should come with a test that would fail without it.
