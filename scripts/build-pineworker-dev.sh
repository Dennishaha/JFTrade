#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/lib/pinets-license.sh"

OUT_DIR="${JFTRADE_PINEWORKER_DEV_OUT_DIR:-$ROOT_DIR/var/pineworker}"
WORKER_ENTRY="$ROOT_DIR/workers/pineworker/src/main.ts"

case "$(uname -s):$(uname -m)" in
  Darwin:arm64)
    BUN_TARGET="bun-darwin-arm64"
    OUTPUT_NAME="worker-darwin-arm64"
    ;;
  Darwin:x86_64)
    BUN_TARGET="bun-darwin-x64"
    OUTPUT_NAME="worker-darwin-x64"
    ;;
  Linux:x86_64)
    BUN_TARGET="bun-linux-x64"
    OUTPUT_NAME="worker-linux-x64"
    ;;
  Linux:aarch64|Linux:arm64)
    BUN_TARGET="bun-linux-arm64"
    OUTPUT_NAME="worker-linux-arm64"
    ;;
  MINGW*:x86_64|MSYS*:x86_64|CYGWIN*:x86_64)
    BUN_TARGET="bun-windows-x64"
    OUTPUT_NAME="worker-windows-x64.exe"
    ;;
  MINGW*:aarch64|MSYS*:aarch64|CYGWIN*:aarch64|MINGW*:arm64|MSYS*:arm64|CYGWIN*:arm64)
    BUN_TARGET="bun-windows-arm64"
    OUTPUT_NAME="worker-windows-arm64.exe"
    ;;
  *)
    echo "unsupported PineTS worker dev platform: $(uname -s)/$(uname -m)" >&2
    exit 1
    ;;
esac

if ! command -v bun >/dev/null 2>&1; then
  echo "bun is not installed or not on PATH" >&2
  exit 1
fi

if ! pinets_check_package_and_license; then
  echo "PineTS dev worker build is blocked until the pinets package is installed." >&2
  exit 1
fi

mkdir -p "$OUT_DIR"
OUT_PATH="$OUT_DIR/$OUTPUT_NAME"
bun build --compile --target="$BUN_TARGET" "$WORKER_ENTRY" --outfile "$OUT_PATH"
chmod +x "$OUT_PATH"

echo "$OUT_PATH"
