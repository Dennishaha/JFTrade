#!/usr/bin/env bash

set -euo pipefail

ALLOW_BLOCKED=0
for arg in "$@"; do
  case "$arg" in
    --allow-blocked)
      ALLOW_BLOCKED=1
      ;;
    *)
      echo "unknown argument: $arg" >&2
      echo "usage: bash scripts/check-pinets-release.sh [--allow-blocked]" >&2
      exit 2
      ;;
  esac
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BLOCKED=0
RUN_LOG="${JFTRADE_PINETS_RELEASE_RUN_LOG:-}"
PINETS_CHECK_STATUS="${JFTRADE_PINETS_RELEASE_PINETS_STATUS:-}"
PINETS_LICENSE="${JFTRADE_PINETS_RELEASE_PINETS_LICENSE:-}"
COMMERCIAL_LICENSE_ACK="${JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK:-}"

run() {
  echo "==> $*"
  if [[ -n "$RUN_LOG" ]]; then
    printf '%s\n' "$*" >> "$RUN_LOG"
  fi
  "$@"
}

mark_blocked() {
  echo "BLOCKED: $*" >&2
  BLOCKED=1
}

echo "==> Checking commercial pinets package"
if [[ -n "$PINETS_CHECK_STATUS" ]]; then
  if [[ "$PINETS_CHECK_STATUS" != "0" ]]; then
    mark_blocked "pinets package is not installed or not visible to npm workspaces"
  fi
elif ! npm ls pinets --workspaces --depth=1; then
  mark_blocked "pinets package is not installed or not visible to npm workspaces"
fi

if [[ -z "$PINETS_LICENSE" && "$BLOCKED" -eq 0 ]]; then
  PINETS_LICENSE="$(node -e "const pkg=require('./node_modules/pinets/package.json'); console.log(pkg.license || '')" 2>/dev/null || true)"
fi
if [[ "$BLOCKED" -eq 0 ]]; then
  echo "==> Checking PineTS commercial license attestation"
  if [[ "$COMMERCIAL_LICENSE_ACK" != "1" ]]; then
    mark_blocked "commercial PineTS license attestation is missing; set JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK=1 only after legal approval"
  fi
  case "$PINETS_LICENSE" in
    ""|"UNLICENSED"|"Commercial"|"commercial"|"SEE LICENSE IN LICENSE"|"SEE LICENSE IN LICENSE.md")
      ;;
    *)
      mark_blocked "pinets package license is ${PINETS_LICENSE}; release requires a recorded commercial license approval"
      ;;
  esac
fi

run go test ./internal/app/apiserver/servercore -run TestResolvePineWorkerRuntimeConfigDefaultsToRealPineTSWorker -v
run go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v
run go test ./pkg/strategy/pineworker -run Test -cover
run go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem
run npm run test:pineworker
run npm run typecheck:pineworker
run npm run build:frontend-assets
run go test -tags release_assets ./internal/frontendassets -run TestFileSystem

if [[ "$BLOCKED" -eq 0 ]]; then
  run env JFTRADE_PINEWORKER_REAL_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerRealPineTSProcessSmoke -v
  run bash scripts/build-pineworker-assets.sh
  run go test -tags release_assets ./internal/pineworkerassets -run Test
else
  echo "==> Skipping real PineTS process smoke and release asset build until pinets is installed"
fi

if [[ "$BLOCKED" -ne 0 && "$ALLOW_BLOCKED" -ne 1 ]]; then
  echo "PineTS release acceptance is blocked; rerun with --allow-blocked only for migration progress checks." >&2
  exit 1
fi

if [[ "$BLOCKED" -ne 0 ]]; then
  echo "PineTS release acceptance gates ran in blocked mode."
else
  echo "PineTS release acceptance gates passed."
fi
