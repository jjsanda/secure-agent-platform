#!/usr/bin/env bash
# Post-process the Python gRPC stubs emitted by `buf generate`.
#
# buf's protocolbuffers/python + grpc/python plugins emit *absolute* imports
# rooted at the proto package, e.g.
#
#     from sap.v1 import common_pb2 as ...
#
# We vendor the stubs under the `sap_worker.gen` package (src layout), so those
# imports must be rewritten to the real Python package path:
#
#     from sap_worker.gen.sap.v1 import common_pb2 as ...
#
# This script also drops the `__init__.py` files that turn the generated tree
# into an importable package. It is idempotent: rewritten lines no longer match
# the source pattern, and existing __init__.py files are left untouched. Run it
# from anywhere; paths are resolved relative to the repo root.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gen_dir="$repo_root/agent-worker/src/sap_worker/gen"

if [[ ! -d "$gen_dir" ]]; then
  echo "fix-python-protos: generated dir not found: $gen_dir" >&2
  exit 1
fi

# 1) Rewrite the absolute `sap.v1` imports to the vendored package path.
#    Anchored at line start so proto-namespace strings ('sap.v1.RunnerService',
#    'sap/v1/runner.proto', serialized descriptors, ...) are never touched.
while IFS= read -r -d '' file; do
  sed -i -E 's/^from sap\.v1 import /from sap_worker.gen.sap.v1 import /' "$file"
done < <(find "$gen_dir" -type f \( -name '*.py' -o -name '*.pyi' \) -print0)

# 2) Ensure the generated tree is an importable package.
for pkg in "$gen_dir" "$gen_dir/sap" "$gen_dir/sap/v1"; do
  init="$pkg/__init__.py"
  if [[ ! -f "$init" ]]; then
    printf '# Generated gRPC stubs package. Populated by `buf generate` + fix-python-protos.sh.\n' >"$init"
  fi
done

echo "fix-python-protos: rewrote sap.v1 imports and ensured __init__.py under ${gen_dir#"$repo_root/"}"
