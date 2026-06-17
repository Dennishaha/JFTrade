#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

# --- Default runtime configuration ---------------------------------------
# Default to the release-style GUI/API ports. Full bbgo engine runs can still use
# `go run ./cmd/jftrade run --config ./config/jftrade.yaml` directly.
export JFTRADE_API_BIND="${JFTRADE_API_BIND:-127.0.0.1:6699}"
export JFTRADE_GUI_BIND="${JFTRADE_GUI_BIND:-127.0.0.1:6688}"
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

echo "Generating Swagger docs / 生成 Swagger 文档..."
npm run generate:openapi

echo "Running frontend typecheck / 运行前端类型检查..."
npm run typecheck

echo "Building frontend / 构建前端..."
npm run build:web

echo "Staging embedded frontend assets / 暂存内嵌前端资源..."
rm -rf "$ROOT_DIR/internal/frontendassets/dist" "$ROOT_DIR/internal/frontendassets/dist.zip"
cp -R "$ROOT_DIR/apps/web/dist" "$ROOT_DIR/internal/frontendassets/dist"
go run ./scripts/archive_frontend_assets.go \
  -src "$ROOT_DIR/apps/web/dist" \
  -dst "$ROOT_DIR/internal/frontendassets/dist.zip"

echo "Starting JFTrade service / 启动 JFTrade 服务..."
echo "JFTrade GUI / 前端地址: http://${JFTRADE_GUI_BIND}"
echo "JFTrade API / 后端地址: http://${JFTRADE_API_BIND}"
go run -tags release_assets ./cmd/jftrade api
