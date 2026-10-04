#!/usr/bin/env bash
# End-to-end demo: bring up the default (zero-secret) stack, then log in as two
# tenants, run an agent, print the streamed trace + audit log, and prove tenant
# isolation. Uses only curl (health poll) + python3 (the flow, stdlib only).
set -euo pipefail

cd "$(dirname "$0")/.."
compose=(docker compose -f deploy/compose/docker-compose.yaml)

echo "==> starting the default profile (mock LLM + mock OIDC, no secrets needed)"
"${compose[@]}" up -d --build

echo "==> waiting for the control plane API to become healthy"
ready=
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:8080/healthz >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
if [[ "${ready}" != 1 ]]; then
  echo "control plane did not become healthy in time; recent logs:" >&2
  "${compose[@]}" logs control-plane | tail -40 >&2
  exit 1
fi

echo "==> running the end-to-end demo"
python3 scripts/demo.py

echo
echo "Tear down with:  make down"
