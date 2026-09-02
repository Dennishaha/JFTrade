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

require_command cargo
require_command rustup
require_command pnpm

echo "Starting JFTrade Tauri development desktop / 启动 JFTrade Tauri 开发桌面..."
echo "The Rust API is the only product API entry; Go is retained for reference and contract generation only."
pnpm install --frozen-lockfile
exec pnpm run dev:desktop
