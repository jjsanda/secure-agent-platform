#!/usr/bin/env bash
# Render every Mermaid source in docs/diagrams/*.mmd to SVG (crisp, for the README)
# and PNG (for contexts that need raster). Uses mermaid-cli via npx — no global install.
set -euo pipefail

cd "$(dirname "$0")/.."
src="docs/diagrams"
out="docs/diagrams/rendered"
cfg="docs/diagrams/mermaid.config.json"
mkdir -p "$out"

shopt -s nullglob
files=("$src"/*.mmd)
if [[ ${#files[@]} -eq 0 ]]; then
  echo "no .mmd files found in $src"; exit 0
fi

cfg_args=()
[[ -f "$cfg" ]] && cfg_args+=(-c "$cfg")
[[ -f "$src/puppeteer.config.json" ]] && cfg_args+=(-p "$src/puppeteer.config.json")

for f in "${files[@]}"; do
  base="$(basename "${f%.mmd}")"
  echo ">> rendering ${base}"
  npx -y @mermaid-js/mermaid-cli -i "$f" -o "$out/${base}.svg" -b transparent "${cfg_args[@]}"
  npx -y @mermaid-js/mermaid-cli -i "$f" -o "$out/${base}.png" -b white -s 2 "${cfg_args[@]}"
done

echo ">> rendered ${#files[@]} diagram(s) -> ${out}"
