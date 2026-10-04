.DEFAULT_GOAL := help
SHELL := /usr/bin/env bash
COMPOSE := docker compose -f deploy/compose/docker-compose.yaml

## help: show available targets
help:
	@echo "secure-agent-platform — make targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | awk -F': ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

## tools: install dev CLIs (buf, sqlc, migrate, golangci-lint)
tools:
	./scripts/install-tools.sh

## keys: generate a dev PASETO v4 keypair and print export lines
keys:
	./scripts/gen-keys.sh

## proto: lint + generate gRPC stubs (Go + Python)
proto:
	cd proto && buf lint && buf generate
	./scripts/fix-python-protos.sh

## up: start the default profile (zero secrets required)
up:
	$(COMPOSE) up -d --build

## demo: one-command end-to-end demo (seed tenants, run an agent, show audit log)
demo:
	./scripts/demo.sh

## down: stop and remove the stack (all profiles, with volumes)
down:
	$(COMPOSE) --profile keycloak --profile observability down -v

## test: run all unit + integration tests (Go + Python)
test:
	cd control-plane && go test -race ./...
	cd agent-worker && uv run pytest

## lint: run linters/formatters in check mode
lint:
	cd control-plane && test -z "$$(gofmt -l .)" && go vet ./... && golangci-lint run ./...
	cd agent-worker && uv run ruff check . && uv run black --check . && uv run mypy src

## fmt: auto-format Go + Python
fmt:
	cd control-plane && gofmt -w .
	cd agent-worker && uv run ruff check --fix . && uv run black .

## diagrams: render Mermaid diagrams to docs/diagrams/rendered/
diagrams:
	./scripts/render-diagrams.sh

.PHONY: help tools keys proto up demo down test lint fmt diagrams
