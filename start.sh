#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

# --- Default runtime configuration ---------------------------------------
# bbgo's session bootstrap requires the Futu OpenD address; default it to the
# FutuOpenD API address; default it to the native OpenD API port so the backend boots out of the box. Override
# by exporting FUTU_OPEND_ADDR before invoking this script.
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
  echo "go 未安装或不在 PATH 中" >&2
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "npm 未安装或不在 PATH 中" >&2
  exit 1
fi

if [[ ! -d node_modules ]]; then
  echo "安装前端依赖..."
  npm install
fi

echo "运行 Go 测试..."
go test ./...

echo "运行前端类型检查..."
npm run typecheck

echo "构建前端..."
npm run build:web

echo "启动后端服务..."
go run ./cmd/jftrade run --config ./config/jftrade.yaml &
BACKEND_PID=$!

cleanup() {
  if kill -0 "$BACKEND_PID" >/dev/null 2>&1; then
    kill "$BACKEND_PID"
  fi
}

trap cleanup EXIT INT TERM

echo "后端 PID: $BACKEND_PID"
echo "JFTrade API: http://${JFTRADE_API_BIND}"
echo "启动前端预览服务: http://127.0.0.1:6688"
npm --workspace @jftrade/web run preview