#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is not installed or not on PATH" >&2
    exit 1
  fi
}

for command in cargo rustup node pnpm; do
  require_command "$command"
done

if [[ -z "${JFTRADE_DESKTOP_RELEASE_TAG:-}" ]]; then
  echo "Set JFTRADE_DESKTOP_RELEASE_TAG=vX.Y.Z before building a release." >&2
  exit 1
fi

echo "Building the Rust/Tauri desktop release for the current host..."
echo "Cross-platform release artifacts are produced by the Tauri CI matrix."
pnpm install --frozen-lockfile
pnpm run check:zero-go
pnpm run build:desktop

echo "Tauri release artifacts are under target/release/bundle."
