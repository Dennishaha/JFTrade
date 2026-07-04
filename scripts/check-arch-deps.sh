#!/usr/bin/env bash
set -euo pipefail

# Architecture dependency checker for JFTrade.
# Blocks forbidden dependency directions that are already expected to hold.

PASS=0
FAIL=0
WARN=0

check_no_import() {
  local from="$1"
  local forbidden="$2"
  local label="$3"
  local imports
  if ! imports="$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' "$from")"; then
    echo "  ❌ $label: unable to inspect $from"
    FAIL=$((FAIL + 1))
  elif rg -F -x -q "$forbidden" <<<"$imports"; then
    echo "  ❌ $label: $from imports $forbidden"
    FAIL=$((FAIL + 1))
  else
    echo "  ✅ $label"
    PASS=$((PASS + 1))
  fi
}

warn_direct_import() {
  local from="$1"
  local forbidden="$2"
  local label="$3"
  local imports

  if ! imports="$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' "$from")"; then
    echo "  ❌ $label: unable to inspect $from"
    FAIL=$((FAIL + 1))
  elif rg -F -x -q "$forbidden" <<<"$imports"; then
    echo "  ⚠️  $label: $from still imports $forbidden"
    WARN=$((WARN + 1))
  else
    echo "  ✅ $label"
    PASS=$((PASS + 1))
  fi
}

check_package_set_no_import() {
  local pattern="$1"
  local forbidden="$2"
  local label="$3"
  local packages
  local found=0

  if ! packages="$(go list "$pattern")"; then
    echo "  ❌ $label: unable to list packages matching $pattern"
    FAIL=$((FAIL + 1))
    return
  fi

  while IFS= read -r pkg; do
    if [ -z "$pkg" ]; then
      continue
    fi
    found=1
    check_no_import "$pkg" "$forbidden" "$label: $pkg → $forbidden"
  done <<<"$packages"

  if [ "$found" -eq 0 ]; then
    echo "  ℹ️  $label: no packages matched $pattern"
  fi
}

check_source_no_match() {
  local path="$1"
  local glob="$2"
  local forbidden="$3"
  local label="$4"
  local matches
  local status

  set +e
  matches="$(rg -n "$forbidden" "$path" --glob "$glob" 2>&1)"
  status=$?
  set -e

  if [ "$status" -eq 0 ]; then
    echo "  ❌ $label: found forbidden source dependency"
    echo "$matches"
    FAIL=$((FAIL + 1))
  elif [ "$status" -eq 1 ]; then
    echo "  ✅ $label"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $label: unable to inspect $path"
    echo "$matches"
    FAIL=$((FAIL + 1))
  fi
}

check_only_standard_library() {
  local package="$1"
  local label="$2"
  local imports
  local non_standard=()

  if ! imports="$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' "$package")"; then
    echo "  ❌ $label: unable to inspect $package"
    FAIL=$((FAIL + 1))
    return
  fi

  while IFS= read -r imported; do
    if [ -z "$imported" ]; then
      continue
    fi
    if [ "$(go list -f '{{.Standard}}' "$imported")" != "true" ]; then
      non_standard+=("$imported")
    fi
  done <<<"$imports"

  if [ "${#non_standard[@]}" -gt 0 ]; then
    echo "  ❌ $label: non-standard imports found"
    printf '    %s\n' "${non_standard[@]}"
    FAIL=$((FAIL + 1))
  else
    echo "  ✅ $label"
    PASS=$((PASS + 1))
  fi
}

echo "=== JFTrade Architecture Dependency Check ==="
echo ""

# Rule 1: internal/api/* must not import Futu SDK or protobuf packages.
echo "Rule 1: internal/api/* must not import Futu integration packages"
check_package_set_no_import "./internal/api/..." "pkg/futu" "api transport must stay broker-protocol free"
check_package_set_no_import "./internal/api/..." "pkg/adk" "api transport must stay ADK-implementation free"
check_package_set_no_import "./internal/api/..." "pkg/backtest" "api transport must stay backtest-implementation free"
check_package_set_no_import "./internal/api/..." "google.golang.org/protobuf" "api transport must stay protobuf free"
check_no_import "github.com/jftrade/jftrade-main/internal/api/live" "pkg/jftradeapi" "live transport → jftradeapi"
check_no_import "github.com/jftrade/jftrade-main/internal/api/live" "pkg/futu" "live transport → Futu"
check_no_import "github.com/jftrade/jftrade-main/internal/api/live" "google.golang.org/protobuf" "live transport → protobuf"
echo ""

# Rule 2: internal/backtest must not import broker protocol packages.
echo "Rule 2: backtest business service must stay broker-protocol free"
check_package_set_no_import "./internal/backtest/..." "pkg/futu" "backtest must not depend on Futu"
check_package_set_no_import "./internal/backtest/..." "google.golang.org/protobuf" "backtest must not depend on protobuf"
echo ""

# Rule 3: internal/api/httpserver must not import pkg/jftradeapi
echo "Rule 3: httpserver must not depend on jftradeapi"
check_no_import "github.com/jftrade/jftrade-main/internal/api/httpserver" "pkg/jftradeapi" "httpserver → jftradeapi"
echo ""

# Rule 4: internal/api/middleware must not import pkg/jftradeapi
echo "Rule 4: middleware must not depend on jftradeapi"
check_no_import "github.com/jftrade/jftrade-main/internal/api/middleware" "pkg/jftradeapi" "middleware → jftradeapi"
echo ""

# Rule 5: pkg/futu must not import pkg/jftradeapi
echo "Rule 5: futu adapter must not depend on sidecar"
check_no_import "github.com/jftrade/jftrade-main/pkg/futu" "pkg/jftradeapi" "futu → jftradeapi"
echo ""

# Rule 6: business modules must not import HTTP transport.
echo "Rule 6: business modules must not depend on internal/api"
for pattern in \
  ./internal/system/... \
  ./internal/settings/... \
  ./internal/datamanagement/... \
  ./internal/marketdata/... \
  ./internal/trading/... \
  ./internal/strategy/... \
  ./internal/backtest/... \
  ./internal/assistant/...
do
  check_package_set_no_import "$pattern" "internal/api" "business module transport boundary"
done
echo ""

# Rule 6a: domain modules must never depend on the application composition root.
echo "Rule 6a: business modules must not depend on servercore"
for pattern in \
  ./internal/system/... \
  ./internal/settings/... \
  ./internal/datamanagement/... \
  ./internal/marketdata/... \
  ./internal/trading/... \
  ./internal/strategy/... \
  ./internal/backtest/... \
  ./internal/assistant/...
do
  check_package_set_no_import "$pattern" "internal/app/apiserver/servercore" "business module composition-root boundary"
done
echo ""

# Rule 6b: assistant layers must not flow back into transport, concrete stores, integrations, or legacy server.
echo "Rule 6b: assistant layers must stay behind their adapter boundaries"
for forbidden in \
  "internal/api/app/store/integration" \
  "internal/store/integration" \
  "internal/integration" \
  "pkg/jftradeapi"
do
  check_package_set_no_import "./internal/assistant/..." "$forbidden" "assistant core boundary"
done
check_package_set_no_import "./internal/api/assistant/..." "pkg/jftradeapi" "assistant transport must not depend on legacy sidecar"
echo ""

# Rule 6c: workflow rules are pure business policy and must not depend on assistant runtime orchestration.
echo "Rule 6c: assistant workflow rules must not depend on runtime orchestration"
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/assistant" \
  "github.com/jftrade/jftrade-main/pkg/jftradeapi"
do
  check_package_set_no_import "./internal/assistant/workflow/..." "$forbidden" "assistant workflow rules boundary"
done
echo ""

# Rule 6d: strategy runtime activity persistence belongs to strategy, not servercore.
echo "Rule 6d: strategy runtime activity store must stay out of servercore"
for forbidden in \
  "internal/api" \
  "internal/app/apiserver/servercore" \
  "pkg/jftradeapi"
do
  check_package_set_no_import "./internal/strategy/runtimeactivity/..." "$forbidden" "strategy runtime activity boundary"
done
echo ""

# Rule 6e: strategy runtime control policy belongs to strategy and stays broker/runtime neutral.
echo "Rule 6e: strategy runtime control rules must stay out of servercore and broker execution"
for forbidden in \
  "internal/api" \
  "internal/app/apiserver/servercore" \
  "internal/trading" \
  "pkg/broker" \
  "pkg/jftradeapi"
do
  check_package_set_no_import "./internal/strategy/runtimecontrol/..." "$forbidden" "strategy runtime control boundary"
done
echo ""

# Rule 6f: strategy instance binding rules belong to strategy and stay catalog/runtime neutral.
echo "Rule 6f: strategy instance binding rules must stay out of servercore and runtime execution"
for forbidden in \
  "internal/api" \
  "internal/app/apiserver/servercore" \
  "internal/trading" \
  "internal/strategy/runtimeactivity" \
  "pkg/broker" \
  "pkg/jftradeapi"
do
  check_package_set_no_import "./internal/strategy/instancebinding/..." "$forbidden" "strategy instance binding boundary"
done
echo ""

# Rule 6g: strategy instance view rules belong to strategy and stay catalog/runtime neutral.
echo "Rule 6g: strategy instance view rules must stay out of servercore and runtime execution"
for forbidden in \
  "internal/api" \
  "internal/app/apiserver/servercore" \
  "internal/trading" \
  "internal/strategy/runtimeactivity" \
  "pkg/broker" \
  "pkg/jftradeapi"
do
  check_package_set_no_import "./internal/strategy/instanceview/..." "$forbidden" "strategy instance view boundary"
done
echo ""

# Rule 7: cmd entrypoints must go through internal/app assembly packages.
echo "Rule 7: cmd entrypoints must not depend on legacy jftradeapi package"
check_package_set_no_import "./cmd/..." "pkg/jftradeapi" "cmd entrypoint boundary"
echo ""

# Rule 8: apiserver owns startup lifecycle and must not forward to legacy startup APIs.
echo "Rule 8: apiserver must not forward startup to legacy jftradeapi entrypoints"
if rg -n "jftradeapi\\.(RunAPIOnly|StartForRunArgs)" internal/app/apiserver --glob '*.go' >/dev/null; then
  echo "  ❌ apiserver startup lifecycle: found direct forwarding to pkg/jftradeapi startup APIs"
  rg -n "jftradeapi\\.(RunAPIOnly|StartForRunArgs)" internal/app/apiserver --glob '*.go'
  FAIL=$((FAIL + 1))
else
  echo "  ✅ apiserver startup lifecycle"
  PASS=$((PASS + 1))
fi
echo ""

# Rule 9: apiserver must not depend on the compatibility facade.
echo "Rule 9: apiserver must not depend on legacy jftradeapi facade"
legacy_helper_pattern="jftradeapi\\.(ResolveLaunchDefaults|EnsureRuntimeLayout|NewSettingsStore|APIBaseURLForBind|PortFromBind|SettingsStore)"
if rg -n "$legacy_helper_pattern" internal/app/apiserver --glob '*.go' >/dev/null; then
  echo "  ❌ apiserver helper boundary: found legacy jftradeapi settings/runtime helper usage"
  rg -n "$legacy_helper_pattern" internal/app/apiserver --glob '*.go'
  FAIL=$((FAIL + 1))
else
  echo "  ✅ apiserver settings/runtime helper boundary"
  PASS=$((PASS + 1))
fi

bad_apiserver_imports="$(rg -l '"github.com/jftrade/jftrade-main/pkg/jftradeapi"' internal/app/apiserver --glob '*.go' || true)"
if [ -n "$bad_apiserver_imports" ]; then
  echo "  ❌ apiserver compatibility facade boundary: unexpected jftradeapi import(s)"
  echo "$bad_apiserver_imports"
  FAIL=$((FAIL + 1))
else
  echo "  ✅ apiserver compatibility facade boundary"
  PASS=$((PASS + 1))
fi
echo ""

# Rule 10: settings persistence must not depend on concrete broker integrations.
echo "Rule 10: settings persistence must stay broker-integration free"
check_package_set_no_import "./internal/store/settingsfile" "pkg/futu" "settingsfile must not depend on Futu"
echo ""

# Rule 11: the backtest adapter boundary belongs to internal/integration/futu.
echo "Rule 11: backtest Futu adapter must stay isolated in the integration layer"
check_source_no_match \
  "internal/app/apiserver/servercore" \
  "backtest_adapter*.go" \
  '"github\.com/jftrade/jftrade-main/pkg/futu|google\.golang\.org/protobuf' \
  "servercore backtest adapters must not import Futu or protobuf"
check_package_set_no_import \
  "./internal/integration/futu/..." \
  "internal/api" \
  "Futu integration must not depend on API transport"
check_package_set_no_import \
  "./internal/integration/futu/..." \
  "pkg/jftradeapi" \
  "Futu integration must not depend on legacy sidecar"
echo ""

# Rule 12: live publisher is a source-neutral standard-library package.
echo "Rule 12: internal/live must depend only on the standard library"
check_only_standard_library \
  "github.com/jftrade/jftrade-main/internal/live" \
  "live publisher standard-library boundary"
echo ""

# Rule 13: trading order orchestration must remain protocol and transport neutral.
echo "Rule 13: internal/trading must stay protocol, legacy-server, and live-transport free"
for forbidden in \
  "pkg/futu" \
  "google.golang.org/protobuf" \
  "pkg/jftradeapi" \
  "internal/live"
do
  check_package_set_no_import \
    "./internal/trading/..." \
    "$forbidden" \
    "trading order orchestration boundary"
done
echo ""

# Rule 14: marketdata owns subscriptions and tick data without transport or legacy/protocol dependencies.
echo "Rule 14: internal/marketdata must stay transport, legacy-server, and Futu-protocol free"
for forbidden in \
  "internal/api" \
  "pkg/jftradeapi" \
  "pkg/futu" \
  "github.com/c9s/bbgo" \
  "google.golang.org/protobuf" \
  "internal/strategy" \
  "pkg/strategy"
do
  check_package_set_no_import \
    "./internal/marketdata/..." \
    "$forbidden" \
    "marketdata data-plane ownership boundary"
done
echo ""

# Rule 15: assistant transport and service boundaries must not regress.
echo "Rule 15: assistant transport and business service boundaries"
for forbidden in \
  "pkg/jftradeapi" \
  "pkg/futu" \
  "google.golang.org/protobuf"
do
  check_package_set_no_import \
    "./internal/api/assistant" \
    "$forbidden" \
    "assistant transport boundary"
done
for forbidden in \
  "internal/api" \
  "internal/app" \
  "internal/store" \
  "internal/integration" \
  "pkg/jftradeapi"
do
  check_package_set_no_import \
    "./internal/assistant/..." \
    "$forbidden" \
    "assistant business service boundary"
done
echo ""

# Rule 16: the retired compatibility facade must not be recreated.
echo "Rule 16: pkg/jftradeapi compatibility facade must stay retired"
legacy_files="$(find pkg/jftradeapi -maxdepth 1 -name '*.go' -print 2>/dev/null || true)"
if [ -z "$legacy_files" ]; then
  echo "  ✅ pkg/jftradeapi has no Go files"
  PASS=$((PASS + 1))
else
  echo "  ❌ pkg/jftradeapi contains Go files; use internal/app/apiserver and domain packages directly"
  echo "$legacy_files"
  FAIL=$((FAIL + 1))
fi
echo ""

# Transitional inventory: these imports are known migration work and become
# hard failures after their internal adapters own the implementation boundary.
echo "Rule 16a: servercore concrete implementation imports (migration warnings)"
for forbidden in \
  "github.com/jftrade/jftrade-main/pkg/futu" \
  "github.com/jftrade/jftrade-main/pkg/adk" \
  "github.com/jftrade/jftrade-main/pkg/backtest"
do
  warn_direct_import \
    "github.com/jftrade/jftrade-main/internal/app/apiserver/servercore" \
    "$forbidden" \
    "servercore concrete implementation boundary"
done
echo ""

echo "=== Results: $PASS passed, $WARN warnings, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
  echo "ERROR: $FAIL forbidden dependency(s) detected."
  exit 1
fi
