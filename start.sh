#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

# --- Default runtime configuration ---------------------------------------
# Default the sidecar to FutuOpenD's native API port. Full bbgo engine runs can
# still use `go run ./cmd/jftrade run --config ./config/jftrade.yaml` directly.
export JFTRADE_API_BIND="${JFTRADE_API_BIND:-127.0.0.1:3000}"
export JFTRADE_FUTU_API_PORT="${JFTRADE_FUTU_API_PORT:-11110}"
export JFTRADE_FUTU_WEBSOCKET_PORT="${JFTRADE_FUTU_WEBSOCKET_PORT:-11111}"
export FUTU_OPEND_ADDR="${FUTU_OPEND_ADDR:-127.0.0.1:${JFTRADE_FUTU_API_PORT}}"

# Stale bbgo market caches (DISABLE_MARKETS_CACHE) can preserve an empty market
# set and break session init; clear cache each boot.
export DISABLE_MARKETS_CACHE="${DISABLE_MARKETS_CACHE:-1}"

# Suppress Node DEP0205 (module.register) deprecation noise emitted by some
# vite plugins. Operators can override NODE_OPTIONS to inspect warnings.
export NODE_OPTIONS="${NODE_OPTIONS:---no-deprecation}"

if ! command -v go >/dev/null 2>&1; then
  echo "go is not installed or not on PATH / go 未安装或不在 PATH 中" >&2
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "npm is not installed or not on PATH / npm 未安装或不在 PATH 中" >&2
  exit 1
fi

echo "Installing frontend dependencies / 安装前端依赖..."
npm install

echo "Running Go tests / 运行 Go 测试..."
go test ./...

echo "Running frontend typecheck / 运行前端类型检查..."
npm run typecheck

echo "Building frontend / 构建前端..."
npm run build:web

echo "Starting backend service / 启动后端服务..."
go run ./cmd/jftrade api &
BACKEND_PID=$!

cleanup() {
  if kill -0 "$BACKEND_PID" >/dev/null 2>&1; then
    kill "$BACKEND_PID"
  fi
}

trap cleanup EXIT INT TERM

echo "Backend PID / 后端 PID: $BACKEND_PID"
echo "JFTrade API / 后端地址: http://${JFTRADE_API_BIND}"
echo "Starting frontend preview service / 启动前端预览服务: http://127.0.0.1:6688"
npm --workspace @jftrade/web run preview