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

check_no_test_import() {
  local from="$1"
  local forbidden="$2"
  local label="$3"
  local imports

  # TestImports and XTestImports contain only imports declared directly by the
  # package's internal and external tests, so transitive dependencies stay out.
  if ! imports="$(go list -f '{{range .TestImports}}{{.}}{{"\n"}}{{end}}{{range .XTestImports}}{{.}}{{"\n"}}{{end}}' "$from")"; then
    echo "  ❌ $label: unable to inspect test imports for $from"
    FAIL=$((FAIL + 1))
  elif rg -F -x -q "$forbidden" <<<"$imports"; then
    echo "  ❌ $label: tests in $from directly import $forbidden"
    FAIL=$((FAIL + 1))
  else
    echo "  ✅ $label"
    PASS=$((PASS + 1))
  fi
}

imports_contain_family() {
  local imports="$1"
  local forbidden="$2"
  local imported

  while IFS= read -r imported; do
    if [[ "$imported" == "$forbidden" || "$imported" == "$forbidden/"* ]]; then
      return 0
    fi
  done <<<"$imports"
  return 1
}

check_no_import_family() {
  local from="$1"
  local forbidden="$2"
  local label="$3"
  local imports
  if ! imports="$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' "$from")"; then
    echo "  ❌ $label: unable to inspect $from"
    FAIL=$((FAIL + 1))
  elif imports_contain_family "$imports" "$forbidden"; then
    echo "  ❌ $label: $from imports package family $forbidden"
    FAIL=$((FAIL + 1))
  else
    echo "  ✅ $label"
    PASS=$((PASS + 1))
  fi
}

check_no_test_import_family() {
  local from="$1"
  local forbidden="$2"
  local label="$3"
  local imports

  if ! imports="$(go list -f '{{range .TestImports}}{{.}}{{"\n"}}{{end}}{{range .XTestImports}}{{.}}{{"\n"}}{{end}}' "$from")"; then
    echo "  ❌ $label: unable to inspect test imports for $from"
    FAIL=$((FAIL + 1))
  elif imports_contain_family "$imports" "$forbidden"; then
    echo "  ❌ $label: tests in $from directly import package family $forbidden"
    FAIL=$((FAIL + 1))
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

check_package_set_no_import_family_except() {
  local pattern="$1"
  local forbidden="$2"
  local excluded="$3"
  local label="$4"
  local packages
  local found=0

  if ! packages="$(go list "$pattern")"; then
    echo "  ❌ $label: unable to list packages matching $pattern"
    FAIL=$((FAIL + 1))
    return
  fi

  while IFS= read -r pkg; do
    if [ -z "$pkg" ] || { [ -n "$excluded" ] && [ "$pkg" = "$excluded" ]; }; then
      continue
    fi
    found=1
    check_no_import_family "$pkg" "$forbidden" "$label: $pkg → $forbidden"
  done <<<"$packages"

  if [ "$found" -eq 0 ]; then
    echo "  ℹ️  $label: no packages matched $pattern"
  fi
}

check_import_family_allowlist() {
  local from="$1"
  local family="$2"
  local label="$3"
  shift 3
  local allowed=("$@")
  local imports
  local offenders=()

  if ! imports="$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' "$from")"; then
    echo "  ❌ $label: unable to inspect $from"
    FAIL=$((FAIL + 1))
    return
  fi

  while IFS= read -r imported; do
    if [[ "$imported" != "$family" && "$imported" != "$family/"* ]]; then
      continue
    fi
    local permitted=0
    local candidate
    for candidate in "${allowed[@]}"; do
      if [ "$imported" = "$candidate" ]; then
        permitted=1
        break
      fi
    done
    if [ "$permitted" -eq 0 ]; then
      offenders+=("$imported")
    fi
  done <<<"$imports"

  if [ "${#offenders[@]}" -gt 0 ]; then
    echo "  ❌ $label: imports outside allowlist"
    printf '    %s\n' "${offenders[@]}"
    FAIL=$((FAIL + 1))
  else
    echo "  ✅ $label"
    PASS=$((PASS + 1))
  fi
}

check_path_absent() {
  local path="$1"
  local label="$2"
  if [ -e "$path" ]; then
    echo "  ❌ $label: legacy path still exists: $path"
    FAIL=$((FAIL + 1))
  else
    echo "  ✅ $label"
    PASS=$((PASS + 1))
  fi
}

check_top_level_directory_allowlist() {
  local root="$1"
  local label="$2"
  shift 2
  local actual
  local expected
  local unexpected
  local missing

  if [ ! -d "$root" ]; then
    echo "  ❌ $label: directory does not exist: $root"
    FAIL=$((FAIL + 1))
    return
  fi

  actual="$(find "$root" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | LC_ALL=C sort)"
  expected="$(printf '%s\n' "$@" | LC_ALL=C sort -u)"
  unexpected="$(comm -13 <(printf '%s\n' "$expected") <(printf '%s\n' "$actual"))"
  missing="$(comm -23 <(printf '%s\n' "$expected") <(printf '%s\n' "$actual"))"

  if [ -n "$unexpected" ] || [ -n "$missing" ]; then
    echo "  ❌ $label: top-level directory set differs from the reviewed public surface"
    if [ -n "$unexpected" ]; then
      echo "    unexpected:"
      sed 's/^/      /' <<<"$unexpected"
    fi
    if [ -n "$missing" ]; then
      echo "    stale allowlist entries:"
      sed 's/^/      /' <<<"$missing"
    fi
    FAIL=$((FAIL + 1))
  else
    echo "  ✅ $label"
    PASS=$((PASS + 1))
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

arch_deps_main() {
echo "=== JFTrade Architecture Dependency Check ==="
echo ""

# Rule 1: internal/api/* must not import Futu SDK or protobuf packages.
echo "Rule 1: internal/api/* must not import Futu integration packages"
check_package_set_no_import "./internal/api/..." "github.com/jftrade/jftrade-main/pkg/futu" "api transport must stay broker-protocol free"
check_package_set_no_import "./internal/api/..." "google.golang.org/protobuf" "api transport must stay protobuf free"
check_no_import "github.com/jftrade/jftrade-main/internal/api/live" "github.com/jftrade/jftrade-main/pkg/futu" "live transport → Futu"
check_no_import "github.com/jftrade/jftrade-main/internal/api/live" "google.golang.org/protobuf" "live transport → protobuf"
echo ""

# Rule 2: internal/backtest must not import broker protocol packages.
echo "Rule 2: backtest business service must stay broker-protocol free"
check_package_set_no_import "./internal/backtest/..." "github.com/jftrade/jftrade-main/pkg/futu" "backtest must not depend on Futu"
check_package_set_no_import "./internal/backtest/..." "google.golang.org/protobuf" "backtest must not depend on protobuf"
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
  ./internal/assistant/... \
  ./internal/watchlist/...
do
  check_package_set_no_import "$pattern" "github.com/jftrade/jftrade-main/internal/api" "business module transport boundary"
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
  ./internal/assistant/... \
  ./internal/watchlist/...
do
  check_package_set_no_import "$pattern" "github.com/jftrade/jftrade-main/internal/app/apiserver/servercore" "business module composition-root boundary"
done
echo ""

# Rule 6b: assistant layers must not flow back into transport, concrete stores, or integrations.
echo "Rule 6b: assistant layers must stay behind their adapter boundaries"
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/api" \
  "github.com/jftrade/jftrade-main/internal/app" \
  "github.com/jftrade/jftrade-main/internal/integration"
do
  check_package_set_no_import_family_except "./internal/assistant/..." "$forbidden" "" "assistant core boundary"
done
check_package_set_no_import_family_except \
  "./internal/assistant/..." \
  "github.com/jftrade/jftrade-main/internal/store" \
  "github.com/jftrade/jftrade-main/internal/assistant/engine" \
  "assistant store boundary"
check_import_family_allowlist \
  "github.com/jftrade/jftrade-main/internal/assistant/engine" \
  "github.com/jftrade/jftrade-main/internal/store" \
  "assistant engine SQLite infrastructure allowlist" \
  "github.com/jftrade/jftrade-main/internal/store/sqliteconn" \
  "github.com/jftrade/jftrade-main/internal/store/sqliteschema"
echo ""

# Rule 6c: workflow rules are pure business policy and must not depend on assistant runtime orchestration.
echo "Rule 6c: assistant workflow rules must not depend on runtime orchestration"
check_package_set_no_import "./internal/assistant/workflow/..." "github.com/jftrade/jftrade-main/internal/assistant" "assistant workflow rules boundary"
echo ""

# Rule 6d: strategy runtime activity persistence belongs to strategy, not servercore.
echo "Rule 6d: strategy runtime activity store must stay out of servercore"
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/api" \
  "github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
do
  check_package_set_no_import "./internal/strategy/runtimeactivity/..." "$forbidden" "strategy runtime activity boundary"
done
echo ""

# Rule 6e: strategy runtime control policy belongs to strategy and stays broker/runtime neutral.
echo "Rule 6e: strategy runtime control rules must stay out of servercore and broker execution"
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/api" \
  "github.com/jftrade/jftrade-main/internal/app/apiserver/servercore" \
  "github.com/jftrade/jftrade-main/internal/trading" \
  "github.com/jftrade/jftrade-main/pkg/broker"
do
  check_package_set_no_import "./internal/strategy/runtimecontrol/..." "$forbidden" "strategy runtime control boundary"
done
echo ""

# Rule 6f: strategy instance binding rules belong to strategy and stay catalog/runtime neutral.
echo "Rule 6f: strategy instance binding rules must stay out of servercore and runtime execution"
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/api" \
  "github.com/jftrade/jftrade-main/internal/app/apiserver/servercore" \
  "github.com/jftrade/jftrade-main/internal/trading" \
  "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity" \
  "github.com/jftrade/jftrade-main/pkg/broker"
do
  check_package_set_no_import "./internal/strategy/instancebinding/..." "$forbidden" "strategy instance binding boundary"
done
echo ""

# Rule 6g: strategy instance view rules belong to strategy and stay catalog/runtime neutral.
echo "Rule 6g: strategy instance view rules must stay out of servercore and runtime execution"
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/api" \
  "github.com/jftrade/jftrade-main/internal/app/apiserver/servercore" \
  "github.com/jftrade/jftrade-main/internal/trading" \
  "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity" \
  "github.com/jftrade/jftrade-main/pkg/broker"
do
  check_package_set_no_import "./internal/strategy/instanceview/..." "$forbidden" "strategy instance view boundary"
done
echo ""

# Rule 6h: the live strategy runtime owns its state machine and consumes only
# broker-neutral/domain ports. Application assembly may import the owner, but
# the owner must never flow back into transport, composition, stores, or Futu.
echo "Rule 6h: live strategy runtime ownership boundary"
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/api" \
  "github.com/jftrade/jftrade-main/internal/app" \
  "github.com/jftrade/jftrade-main/internal/store" \
  "github.com/jftrade/jftrade-main/internal/integration" \
  "github.com/jftrade/jftrade-main/pkg/futu" \
  "github.com/gin-gonic/gin" \
  "google.golang.org/protobuf"
do
  check_package_set_no_import \
    "./internal/strategy/liveruntime/..." \
    "$forbidden" \
    "live strategy runtime owner boundary"
done
check_source_no_match \
  "internal/app/apiserver/servercore" \
  "*.go" \
  'type (strategyRuntimeManager|managedStrategyRuntime|strategySymbolRuntime) struct|func \([^)]*\) (startStrategy|stopStrategy|handleMarketTrade|syncClosedKLinesLoop)\(' \
  "servercore must not regain the live strategy state machine"
echo ""

# Rule 10: settings persistence must not depend on concrete broker integrations.
echo "Rule 10: settings persistence must stay broker-integration free"
check_package_set_no_import "./internal/store/settingsfile" "github.com/jftrade/jftrade-main/pkg/futu" "settingsfile must not depend on Futu"
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
  "github.com/jftrade/jftrade-main/internal/api" \
  "Futu integration must not depend on API transport"
echo ""

# Rule 12: live publisher is a source-neutral standard-library package.
echo "Rule 12: internal/live must depend only on the standard library"
check_only_standard_library \
  "github.com/jftrade/jftrade-main/internal/live" \
  "live publisher standard-library boundary"
echo ""

# Rule 13: trading order orchestration must remain protocol and transport neutral.
echo "Rule 13: internal/trading must stay protocol and live-transport free"
for forbidden in \
  "github.com/jftrade/jftrade-main/pkg/futu" \
  "google.golang.org/protobuf" \
  "github.com/jftrade/jftrade-main/internal/live"
do
  check_package_set_no_import \
    "./internal/trading/..." \
    "$forbidden" \
    "trading order orchestration boundary"
done
echo ""

# Rule 14: marketdata owns subscriptions and tick data without transport or protocol dependencies.
echo "Rule 14: internal/marketdata must stay transport and Futu-protocol free"
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/api" \
  "github.com/jftrade/jftrade-main/pkg/futu" \
  "github.com/c9s/bbgo" \
  "google.golang.org/protobuf" \
  "github.com/jftrade/jftrade-main/internal/strategy" \
  "github.com/jftrade/jftrade-main/pkg/strategy"
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
  "github.com/jftrade/jftrade-main/pkg/futu" \
  "google.golang.org/protobuf"
do
  check_package_set_no_import \
    "./internal/api/assistant" \
    "$forbidden" \
    "assistant transport boundary"
done
for forbidden in \
  "github.com/jftrade/jftrade-main/internal/api" \
  "github.com/jftrade/jftrade-main/internal/app" \
  "github.com/jftrade/jftrade-main/internal/integration"
do
  check_package_set_no_import_family_except \
    "./internal/assistant/..." \
    "$forbidden" \
    "" \
    "assistant business service boundary"
done
check_package_set_no_import_family_except \
  "./internal/assistant/..." \
  "github.com/jftrade/jftrade-main/internal/store" \
  "github.com/jftrade/jftrade-main/internal/assistant/engine" \
  "assistant business store boundary"
check_import_family_allowlist \
  "github.com/jftrade/jftrade-main/internal/assistant/engine" \
  "github.com/jftrade/jftrade-main/internal/store" \
  "assistant engine persistence dependency allowlist" \
  "github.com/jftrade/jftrade-main/internal/store/sqliteconn" \
  "github.com/jftrade/jftrade-main/internal/store/sqliteschema"
echo ""

# These implementation packages now have explicit internal owners. Keep the
# composition root from regaining concrete protocol/runtime dependencies.
echo "Rule 16: servercore concrete implementation imports"
for forbidden in \
  "github.com/jftrade/jftrade-main/pkg/futu" \
  "github.com/jftrade/jftrade-main/internal/assistant/engine" \
  "github.com/jftrade/jftrade-main/pkg/backtest"
do
  check_no_import_family \
    "github.com/jftrade/jftrade-main/internal/app/apiserver/servercore" \
    "$forbidden" \
    "servercore concrete implementation boundary"
done
echo ""

# Keep servercore tests on the same internal boundaries as production code.
echo "Rule 16a: servercore direct test imports"
for forbidden in \
  "github.com/jftrade/jftrade-main/pkg/futu" \
  "github.com/jftrade/jftrade-main/internal/assistant/engine" \
  "github.com/jftrade/jftrade-main/pkg/backtest"
do
  check_no_test_import_family \
    "github.com/jftrade/jftrade-main/internal/app/apiserver/servercore" \
    "$forbidden" \
    "servercore test-only implementation dependency"
done
echo ""

# Hard-cut internal packages must not reappear under pkg/.
echo "Rule 16b: internal package namespace hard cut"
check_top_level_directory_allowlist \
  "pkg" \
  "reviewed public package set" \
  "backtest" \
  "bbgo" \
  "besteffort" \
  "broker" \
  "chart" \
  "futu" \
  "market" \
  "observability" \
  "researchscreen" \
  "strategy"
check_path_absent "pkg/adk" "legacy pkg/adk directory removed"
check_path_absent "pkg/jftsettings" "legacy pkg/jftsettings directory removed"
check_path_absent "pkg/jftradeapi" "legacy pkg/jftradeapi directory remains removed"
check_source_no_match \
  "." \
  "*.go" \
  'github\.com/jftrade/jftrade-main/pkg/(adk|jftsettings)' \
  "legacy internalized package imports"
echo ""

# Pine process lifecycle is infrastructure for the strategy domain. Live order
# semantics belong to internal/strategy, while replay and matching remain
# behind internal/backtest; the runtime package must not bridge those domains.
echo "Rule 17: Pine runtime must stay backtest-independent"
check_no_import \
  "github.com/jftrade/jftrade-main/internal/strategy/pineruntime" \
  "github.com/jftrade/jftrade-main/pkg/backtest" \
  "Pine runtime → pkg/backtest"
check_no_import \
  "github.com/jftrade/jftrade-main/internal/strategy/pineruntime" \
  "github.com/jftrade/jftrade-main/internal/backtest" \
  "Pine runtime → internal/backtest"
echo ""

echo "=== Results: $PASS passed, $WARN warnings, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
  echo "ERROR: $FAIL forbidden dependency(s) detected."
  exit 1
fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  arch_deps_main "$@"
fi
