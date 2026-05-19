#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

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
echo "启动前端预览服务: http://127.0.0.1:4173"
npm --workspace @jftrade/web run preview