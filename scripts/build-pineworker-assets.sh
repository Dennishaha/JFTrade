#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/lib/pinets-license.sh"

OUT_DIR="${JFTRADE_PINEWORKER_ASSET_OUT_DIR:-$ROOT_DIR/internal/pineworkerassets/assets/bin}"
WORKER_ENTRY="$ROOT_DIR/workers/pineworker/src/main.ts"

TARGETS=(
  "bun-darwin-arm64:worker-darwin-arm64"
  "bun-darwin-x64:worker-darwin-x64"
  "bun-linux-x64:worker-linux-x64"
  "bun-linux-arm64:worker-linux-arm64"
  "bun-windows-x64:worker-windows-x64.exe"
  "bun-windows-arm64:worker-windows-arm64.exe"
)

if ! command -v bun >/dev/null 2>&1; then
  echo "bun is not installed or not on PATH" >&2
  exit 1
fi

if ! pinets_check_package_and_license; then
  echo "PineTS worker asset build is blocked until the commercial pinets package/license is available." >&2
  exit 1
fi

mkdir -p "$OUT_DIR"
find "$OUT_DIR" -maxdepth 1 -type f ! -name ".gitkeep" -delete

for target in "${TARGETS[@]}"; do
  IFS=':' read -r bun_target output_name <<<"$target"
  echo "Building PineTS worker ${bun_target} -> ${output_name}"
  bun build --compile --target="$bun_target" "$WORKER_ENTRY" --outfile "$OUT_DIR/$output_name"
done
