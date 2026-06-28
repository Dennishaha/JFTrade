#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

npm run build:web

rm -rf internal/frontendassets/dist
cp -R apps/web/dist internal/frontendassets/dist
go run ./scripts/archive_frontend_assets.go \
  -src internal/frontendassets/dist \
  -dst internal/frontendassets/dist.zip
