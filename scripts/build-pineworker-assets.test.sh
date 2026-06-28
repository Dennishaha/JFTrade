#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

BIN_DIR="$TEMP_DIR/bin"
RUN_LOG="$TEMP_DIR/run.log"
ASSET_OUT_DIR="$TEMP_DIR/assets/bin"
mkdir -p "$BIN_DIR"

stub() {
  local name="$1"
  shift
  {
    printf '#!/bin/sh\n'
    printf 'echo "%s $*" >> "%s"\n' "$name" "$RUN_LOG"
    printf '%s\n' "$*"
  } > "$BIN_DIR/$name"
  chmod +x "$BIN_DIR/$name"
}

stub bun 'exit 0'
stub npm 'exit 0'
stub node 'exit 0'

export PATH="$BIN_DIR:$PATH"
export JFTRADE_PINEWORKER_ASSET_OUT_DIR="$ASSET_OUT_DIR"

export JFTRADE_PINETS_RELEASE_PINETS_STATUS=1
if /bin/bash scripts/build-pineworker-assets.sh >/dev/null 2>"$TEMP_DIR/missing.err"; then
  echo "worker asset build passed despite missing pinets" >&2
  exit 1
fi
if ! grep -q "PineTS worker asset build is blocked until the pinets package is installed" "$TEMP_DIR/missing.err"; then
  echo "worker asset build did not report missing package blocker" >&2
  cat "$TEMP_DIR/missing.err" >&2
  exit 1
fi

: > "$RUN_LOG"
export JFTRADE_PINETS_RELEASE_PINETS_STATUS=0
export JFTRADE_PINETS_RELEASE_PINETS_LICENSE=AGPL-3.0-only
/bin/bash scripts/build-pineworker-assets.sh >"$TEMP_DIR/pass.out" 2>"$TEMP_DIR/pass.err"
if ! grep -q "bun build" "$RUN_LOG"; then
  echo "worker asset build with installed pinets did not invoke bun build" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi
if ! grep -q "pinets package license: AGPL-3.0-only" "$TEMP_DIR/pass.out"; then
  echo "worker asset build did not report pinets package license" >&2
  cat "$TEMP_DIR/pass.out" >&2
  cat "$TEMP_DIR/pass.err" >&2
  exit 1
fi
