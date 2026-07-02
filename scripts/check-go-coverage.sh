#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
profile="$(mktemp "${TMPDIR:-/tmp}/jftrade-go-coverage.XXXXXX")"
threshold="${GO_COVERAGE_THRESHOLD:-85}"

cleanup() {
  rm -f "$profile"
}
trap cleanup EXIT

cd "$repo_root"
go test ./... -count=1 -timeout "${GO_TEST_TIMEOUT:-300s}" -coverprofile="$profile"

raw_total="$(go tool cover -func="$profile" | tail -1 | awk '{print $NF}')"
read -r covered total percentage < <(
  awk '
    NR == 1 { next }
    $1 ~ /\/cmd\// { next }
    $1 ~ /\/docs\/swagger\// { next }
    $1 ~ /\/scripts\// { next }
    $1 ~ /\/internal\/buildinfo\// { next }
    $1 ~ /\/internal\/frontendassets\// { next }
    $1 ~ /\/internal\/pineworkerassets\// { next }
    $1 ~ /\/pkg\/futu\/pb\// { next }
    $1 ~ /\/pkg\/strategy\/pineworker\/pineworkerpb\// { next }
    {
      total += $2
      if ($3 > 0) covered += $2
    }
    END {
      if (total == 0) exit 2
      printf "%d %d %.4f\n", covered, total, covered * 100 / total
    }
  ' "$profile"
)

printf 'Go coverage: raw=%s business=%.2f%% (%s/%s statements) threshold=%s%%\n' \
  "$raw_total" "$percentage" "$covered" "$total" "$threshold"

awk -v actual="$percentage" -v required="$threshold" 'BEGIN {
  if (actual + 0 < required + 0) {
    printf "Go business coverage %.2f%% is below %.2f%%\n", actual, required > "/dev/stderr"
    exit 1
  }
}'

critical_scopes=(
  "internal/api/backtest"
  "internal/api/httpserver"
  "internal/api/live"
  "internal/api/middleware"
  "internal/api/settings"
  "internal/api/system"
  "internal/app/apiserver/lifecycle"
  "internal/store/sqliteschema"
  "pkg/futu/opend"
  "pkg/strategy/ir"
  "pkg/strategy/pineworker"
)

for scope in "${critical_scopes[@]}"; do
  read -r scope_covered scope_total scope_percentage < <(
    awk -v scope="$scope" '
      NR == 1 { next }
      {
        file = $1
        sub(/:.*/, "", file)
        marker = "/" scope "/"
        marker_at = index(file, marker)
        if (marker_at == 0) next
        remainder = substr(file, marker_at + length(marker))
        if (remainder ~ /\//) next
        total += $2
        if ($3 > 0) covered += $2
      }
      END {
        if (total == 0) exit 2
        printf "%d %d %.4f\n", covered, total, covered * 100 / total
      }
    ' "$profile"
  )
  printf 'Critical Go coverage: %-42s %.2f%% (%s/%s)\n' \
    "$scope" "$scope_percentage" "$scope_covered" "$scope_total"
  awk -v scope="$scope" -v actual="$scope_percentage" 'BEGIN {
    if (actual + 0 < 95) {
      printf "Critical Go coverage for %s is %.2f%%, below 95%%\n", scope, actual > "/dev/stderr"
      exit 1
    }
  }'
done
