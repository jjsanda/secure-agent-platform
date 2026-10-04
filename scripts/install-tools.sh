#!/usr/bin/env bash
# Install the developer CLIs this repo uses. Go-based tools go into $(go env GOPATH)/bin.
# mermaid-cli is invoked via `npx` and needs no global install.
set -euo pipefail

# Build every tool with the Go toolchain the project targets (see go.mod). This
# also ensures golangci-lint is built with a Go >= the module's version, so it
# can analyze the code. `go` fetches this toolchain automatically if missing.
export GOTOOLCHAIN=go1.25.11

bin="$(go env GOPATH)/bin"
echo ">> installing Go-based dev tools into ${bin}"

go install github.com/bufbuild/buf/cmd/buf@v1.47.2
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0

echo
echo ">> installed:"
for t in buf sqlc migrate golangci-lint; do
  if [[ -x "${bin}/${t}" ]]; then printf '   %-16s %s\n' "${t}" "$("${bin}/${t}" --version 2>/dev/null | head -n1 || echo ok)"; fi
done
echo
echo ">> ensure ${bin} is on your PATH:  export PATH=\"\$PATH:${bin}\""
echo ">> mermaid-cli is used on demand via: npx @mermaid-js/mermaid-cli"
