#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

BIN_DIR="$TEMP_DIR/bin"
RUN_LOG="$TEMP_DIR/run.log"
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

stub go 'exit 0'
stub npm 'exit 0'
stub bash 'exit 0'

export PATH="$BIN_DIR:$PATH"
export JFTRADE_PINETS_RELEASE_RUN_LOG="$RUN_LOG"
export JFTRADE_PINETS_RELEASE_PINETS_STATUS=1
export JFTRADE_PINETS_RELEASE_PINETS_LICENSE=AGPL-3.0-only
unset JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK

if /bin/bash scripts/check-pinets-release.sh >/dev/null 2>"$TEMP_DIR/strict.err"; then
  echo "strict release check passed despite missing pinets" >&2
  exit 1
fi
if ! grep -q "PineTS release acceptance is blocked" "$TEMP_DIR/strict.err"; then
  echo "strict release check did not report blocked acceptance" >&2
  cat "$TEMP_DIR/strict.err" >&2
  exit 1
fi

: > "$RUN_LOG"
/bin/bash scripts/check-pinets-release.sh --allow-blocked >/dev/null 2>"$TEMP_DIR/allow.err"
if grep -q "build-pineworker-assets" "$RUN_LOG"; then
  echo "blocked release check should skip release asset build" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi
if ! grep -q "go test ./pkg/strategy/pineworker -run Test -cover" "$RUN_LOG"; then
  echo "blocked release check did not run focused Pine worker coverage gate" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi
if ! grep -q "npm run build:frontend-assets" "$RUN_LOG"; then
  echo "blocked release check did not rebuild frontend release assets" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi
if ! grep -q "go test -tags release_assets ./internal/frontendassets -run TestFileSystem" "$RUN_LOG"; then
  echo "blocked release check did not test embedded frontend assets" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi

: > "$RUN_LOG"
export JFTRADE_PINETS_RELEASE_PINETS_STATUS=0
unset JFTRADE_PINETS_RELEASE_PINETS_LICENSE
unset JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK
if /bin/bash scripts/check-pinets-release.sh >/dev/null 2>"$TEMP_DIR/license.err"; then
  echo "release check passed without commercial license attestation" >&2
  exit 1
fi
if ! grep -q "commercial PineTS license attestation is missing" "$TEMP_DIR/license.err"; then
  echo "release check did not report missing commercial license attestation" >&2
  cat "$TEMP_DIR/license.err" >&2
  exit 1
fi
if grep -q "build-pineworker-assets" "$RUN_LOG"; then
  echo "license-blocked release check should skip release asset build" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi

: > "$RUN_LOG"
export JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK=1
export JFTRADE_PINETS_RELEASE_PINETS_LICENSE=AGPL-3.0-only
if /bin/bash scripts/check-pinets-release.sh >/dev/null 2>"$TEMP_DIR/agpl.err"; then
  echo "release check passed with AGPL pinets license" >&2
  exit 1
fi
if ! grep -q "pinets package license is AGPL-3.0-only" "$TEMP_DIR/agpl.err"; then
  echo "release check did not report non-commercial pinets license" >&2
  cat "$TEMP_DIR/agpl.err" >&2
  exit 1
fi

: > "$RUN_LOG"
export JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK=1
export JFTRADE_PINETS_RELEASE_PINETS_LICENSE=Commercial
/bin/bash scripts/check-pinets-release.sh >/dev/null 2>"$TEMP_DIR/pass.err"
if ! grep -q "bash scripts/build-pineworker-assets.sh" "$RUN_LOG"; then
  echo "unblocked release check did not build worker assets" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi
if ! grep -q "env JFTRADE_PINEWORKER_REAL_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerRealPineTSProcessSmoke -v" "$RUN_LOG"; then
  echo "unblocked release check did not run real PineTS process smoke" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi
if ! grep -q "go build -tags release_assets ./cmd/jftrade-api" "$RUN_LOG"; then
  echo "unblocked release check did not build release_assets API binary" >&2
  cat "$RUN_LOG" >&2
  exit 1
fi
