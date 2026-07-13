#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

# --- Default runtime configuration ---------------------------------------
# The release backend serves the embedded frontend and API on one HTTP port.
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

if ! command -v pnpm >/dev/null 2>&1; then
  echo "pnpm is not installed or not on PATH / pnpm 未安装或不在 PATH 中" >&2
  exit 1
fi

echo "Installing frontend dependencies / 安装前端依赖..."
pnpm install --frozen-lockfile

echo "Generating Swagger docs / 生成 Swagger 文档..."
pnpm run generate:openapi

echo "Running frontend typecheck / 运行前端类型检查..."
pnpm run typecheck

echo "Building frontend / 构建前端..."
pnpm run build:web

echo "Staging embedded frontend assets / 暂存内嵌前端资源..."
rm -rf "$ROOT_DIR/internal/frontendassets/dist" "$ROOT_DIR/internal/frontendassets/dist.zip"
cp -R "$ROOT_DIR/apps/web/dist" "$ROOT_DIR/internal/frontendassets/dist"
go run ./scripts/archive_frontend_assets.go \
  -src "$ROOT_DIR/apps/web/dist" \
  -dst "$ROOT_DIR/internal/frontendassets/dist.zip"

echo "Building embedded PineTS worker assets / 构建内嵌 PineTS worker 资源..."
pnpm run build:pineworker

echo "Starting JFTrade service / 启动 JFTrade 服务..."
echo "Optional Web address (disabled by default) / 可选 Web 地址（默认关闭）: http://${JFTRADE_GUI_BIND}"
echo "Enable Web access and set its password in JFTrade Dev > Settings first / 请先在 JFTrade Dev 的设置中开启 Web 访问并设置密码"
go run -tags release_assets ./cmd/jftrade-api
