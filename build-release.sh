#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

EMBED_DIR="$ROOT_DIR/internal/frontendassets/dist"
EMBED_ARCHIVE="$ROOT_DIR/internal/frontendassets/dist.zip"
WEB_DIST_DIR="$ROOT_DIR/apps/web/dist"
OUTPUT_DIR="$ROOT_DIR/dist"
TARGETS=(
  "darwin/arm64"
  "linux/amd64"
  "windows/amd64"
  "windows/arm64"
)

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is not installed or not on PATH" >&2
    exit 1
  fi
}

resolve_version() {
  if [[ -n "${JFTRADE_VERSION:-}" ]]; then
    printf '%s' "$JFTRADE_VERSION"
    return
  fi
  if command -v git >/dev/null 2>&1 && git describe --tags --always --dirty >/dev/null 2>&1; then
    git describe --tags --always --dirty
    return
  fi
  printf '%s' 'dev'
}

resolve_commit() {
  if [[ -n "${JFTRADE_COMMIT:-}" ]]; then
    printf '%s' "$JFTRADE_COMMIT"
    return
  fi
  if command -v git >/dev/null 2>&1 && git rev-parse --short HEAD >/dev/null 2>&1; then
    git rev-parse --short HEAD
    return
  fi
  printf '%s' 'unknown'
}

install_frontend_dependencies() {
  if [[ -f "$ROOT_DIR/package-lock.json" ]]; then
    npm ci
    return
  fi
  npm install
}

require_command go
require_command npm

VERSION="$(resolve_version)"
COMMIT="$(resolve_commit)"
BUILD_TIME="${JFTRADE_BUILD_TIME:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"
BUILD_TAGS="release_assets,netgo,osusergo"
BUILD_LDFLAGS="-s -w -X github.com/jftrade/jftrade-main/internal/buildinfo.Version=$VERSION -X github.com/jftrade/jftrade-main/internal/buildinfo.Commit=$COMMIT -X github.com/jftrade/jftrade-main/internal/buildinfo.BuildTime=$BUILD_TIME"
BUILD_TARGET="./cmd/jftrade-api"
ARTIFACT_PREFIX="jftrade"

echo "Installing frontend dependencies..."
install_frontend_dependencies

echo "Building frontend bundle..."
npm run build:web

echo "Staging embedded frontend assets..."
rm -rf "$EMBED_DIR" "$EMBED_ARCHIVE" "$OUTPUT_DIR"
mkdir -p "$(dirname "$EMBED_DIR")" "$OUTPUT_DIR"
cp -R "$WEB_DIST_DIR" "$EMBED_DIR"
go run ./scripts/archive_frontend_assets.go -src "$WEB_DIST_DIR" -dst "$EMBED_ARCHIVE"

for target in "${TARGETS[@]}"; do
  IFS='/' read -r goos goarch <<<"$target"
  artifact_dir="$OUTPUT_DIR/${ARTIFACT_PREFIX}-${VERSION}-${goos}-${goarch}"
  mkdir -p "$artifact_dir"

  output_name="$ARTIFACT_PREFIX"
  if [[ "$goos" == "windows" ]]; then
    output_name="$ARTIFACT_PREFIX.exe"
  fi

  echo "Building api-only ${goos}/${goarch}..."
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
    -trimpath \
    -buildvcs=false \
    -tags "$BUILD_TAGS" \
    -ldflags "$BUILD_LDFLAGS" \
    -o "$artifact_dir/$output_name" \
    "$BUILD_TARGET"
done

echo "Release artifacts written to $OUTPUT_DIR"