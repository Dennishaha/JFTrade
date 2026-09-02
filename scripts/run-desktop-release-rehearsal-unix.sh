#!/usr/bin/env bash
set -euo pipefail

platform="${1:?platform is required}"
candidate_root="${2:?candidate artifact root is required}"
baseline_package="${3:?baseline package is required}"
report_path="${4:?report path is required}"

case "$platform" in
  macos-arm64|linux-x64) ;;
  *) echo "unsupported Unix rehearsal platform: $platform" >&2; exit 1 ;;
esac

work_root="${RUNNER_TEMP:?RUNNER_TEMP is required}/jftrade-release-rehearsal-$platform"
install_root="$work_root/install"
backup_root="$work_root/backup"
runtime_home="$work_root/home"
rm -rf "$work_root"
mkdir -p "$install_root" "$runtime_home" "$(dirname "$report_path")"
runtime_data_home="$runtime_home/.local/share"

probe_runtime() {
  local executable="$1" label="$2"
  local -a command=(env "HOME=$runtime_home" "XDG_DATA_HOME=$runtime_data_home" "$executable")
  if [[ "$platform" == "linux-x64" ]]; then
    command=(env "HOME=$runtime_home" "XDG_DATA_HOME=$runtime_data_home" xvfb-run -a "$executable")
  fi
  "${command[@]}" >"$work_root/$label.log" 2>&1 &
  local pid=$!
  trap 'kill "$pid" 2>/dev/null || true' RETURN
  local ready=0
  for _ in $(seq 1 120); do
    status="$(curl --silent --show-error --max-time 2 --output "$work_root/$label-status.json" --write-out '%{http_code}' http://127.0.0.1:6699/api/v1/system/status || true)"
    if [[ "$status" == "200" || "$status" == "401" ]]; then
      ready=1
      break
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      wait "$pid" || true
      echo "$label exited before runtime readiness" >&2
      sed -n '1,200p' "$work_root/$label.log" >&2
      return 1
    fi
    sleep 0.25
  done
  [[ "$ready" == 1 ]] || { echo "$label did not become ready" >&2; return 1; }
  jq -e 'type == "object"' "$work_root/$label-status.json" >/dev/null
  if [[ "$label" == "candidate-upgrade" ]]; then
    [[ "$status" == "401" ]] || { echo "candidate API did not fail closed for an unauthenticated request" >&2; return 1; }
    jq -e '.error.code == "WEB_AUTH_REQUIRED"' "$work_root/$label-status.json" >/dev/null
  fi
  kill "$pid" 2>/dev/null || true
  wait "$pid" || true
  for _ in $(seq 1 40); do
    if ! curl --silent --max-time 1 http://127.0.0.1:6699/api/v1/system/status >/dev/null 2>&1; then
      trap - RETURN
      return 0
    fi
    sleep 0.25
  done
  echo "$label left the API listener running after shutdown" >&2
  return 1
}

if [[ "$platform" == "linux-x64" ]]; then
  candidate_package="$(find "$candidate_root" -type f -name '*.AppImage' -print -quit)"
  [[ -n "$candidate_package" ]]
  baseline_install="$install_root/JFTrade-baseline.AppImage"
  candidate_install="$install_root/JFTrade-candidate.AppImage"
  cp "$baseline_package" "$baseline_install"
  chmod +x "$baseline_install"
  probe_runtime "$baseline_install" baseline-first-start
  data_root="$runtime_data_home/jftrade"
  [[ -d "$data_root" ]] || { echo "baseline did not create Linux data root" >&2; exit 1; }
  cp -R "$data_root" "$backup_root"
  cp "$candidate_package" "$candidate_install"
  chmod +x "$candidate_install"
  probe_runtime "$candidate_install" candidate-upgrade
  upgrade_check_root="$work_root/upgraded-data"
  cp -R "$data_root" "$upgrade_check_root"
  rm -f "$candidate_install"
  rm -rf "$data_root"
  cp -R "$backup_root" "$data_root"
  probe_runtime "$baseline_install" baseline-rollback
  rm -f "$baseline_install"
else
  candidate_package="$(find "$candidate_root" -type f -name '*.dmg' -print -quit)"
  [[ -n "$candidate_package" ]]
  mount_dmg() {
    local dmg="$1" mount="$2"
    mkdir -p "$mount"
    hdiutil attach "$dmg" -nobrowse -readonly -mountpoint "$mount" >/dev/null
  }
  baseline_mount="$work_root/baseline-mount"
  candidate_mount="$work_root/candidate-mount"
  mount_dmg "$baseline_package" "$baseline_mount"
  baseline_app="$(find "$baseline_mount" -maxdepth 1 -type d -name '*.app' -print -quit)"
  [[ -n "$baseline_app" ]]
  ditto "$baseline_app" "$install_root/JFTrade.app"
  hdiutil detach "$baseline_mount" >/dev/null
  baseline_executable="$(find "$install_root/JFTrade.app/Contents/MacOS" -maxdepth 1 -type f -perm -111 -print -quit)"
  [[ -n "$baseline_executable" ]]
  probe_runtime "$baseline_executable" baseline-first-start
  data_root="$runtime_home/Library/Application Support/JFTrade"
  [[ -d "$data_root" ]] || { echo "baseline did not create macOS data root" >&2; exit 1; }
  cp -R "$data_root" "$backup_root"
  mount_dmg "$candidate_package" "$candidate_mount"
  candidate_app="$(find "$candidate_mount" -maxdepth 1 -type d -name '*.app' -print -quit)"
  [[ -n "$candidate_app" ]]
  rm -rf "$install_root/JFTrade.app"
  ditto "$candidate_app" "$install_root/JFTrade.app"
  hdiutil detach "$candidate_mount" >/dev/null
  candidate_executable="$install_root/JFTrade.app/Contents/MacOS/jftrade-desktop"
  [[ -x "$candidate_executable" ]]
  probe_runtime "$candidate_executable" candidate-upgrade
  upgrade_check_root="$work_root/upgraded-data"
  cp -R "$data_root" "$upgrade_check_root"
  rm -rf "$install_root/JFTrade.app"
  rm -rf "$data_root"
  cp -R "$backup_root" "$data_root"
  mount_dmg "$baseline_package" "$baseline_mount"
  baseline_app="$(find "$baseline_mount" -maxdepth 1 -type d -name '*.app' -print -quit)"
  ditto "$baseline_app" "$install_root/JFTrade.app"
  hdiutil detach "$baseline_mount" >/dev/null
  baseline_executable="$(find "$install_root/JFTrade.app/Contents/MacOS" -maxdepth 1 -type f -perm -111 -print -quit)"
  probe_runtime "$baseline_executable" baseline-rollback
  rm -rf "$install_root/JFTrade.app"
fi

python3 - "$upgrade_check_root" <<'PY'
import pathlib, sqlite3, sys
root = pathlib.Path(sys.argv[1])
expected = {
    "backtest.db", "backtest-runs.db", "strategy-runtime.db", "execution-orders.db",
    "adk.db", "adk-session.db", "adk-artifact.db", "watchlists.db", "research.db",
}
actual = {path.name for path in root.glob("*.db")}
if actual != expected:
    raise SystemExit(f"expected exactly nine managed databases, got {sorted(actual)}")
for name in sorted(expected):
    connection = sqlite3.connect(f"file:{root / name}?mode=ro", uri=True)
    try:
        result = connection.execute("PRAGMA integrity_check").fetchone()
        if result != ("ok",): raise SystemExit(f"{name} integrity_check failed: {result}")
    finally:
        connection.close()
PY

sbom="$(find "$candidate_root" -type f \( -name '*.spdx.json' -o -name '*sbom*.json' \) -print -quit)"
[[ -n "$sbom" ]] || { echo "candidate artifact has no SBOM" >&2; exit 1; }
pnpm run check:zero-go -- --artifact "$candidate_package" --artifact "$sbom"

export REHEARSAL_PLATFORM="$platform"
export REHEARSAL_ARTIFACT_NAME REHEARSAL_ARTIFACT_ID REHEARSAL_ARTIFACT_DIGEST
export REHEARSAL_CANDIDATE_REF REHEARSAL_PLANNED_TAG REHEARSAL_COMMIT_SHA
node --input-type=module - "$report_path" <<'NODE'
import fs from "node:fs";
const checks = Object.fromEntries([
  "package", "install", "firstStart", "upgrade", "databaseUpgrade", "runtimeSmoke",
  "uninstall", "backupRestore", "rollback", "zeroGo", "sbomZeroGo",
].map((name) => [name, "passed"]));
const report = {
  schemaVersion: "jftrade.release-rehearsal-platform.v1",
  qualificationMode: "rehearsal",
  platform: process.env.REHEARSAL_PLATFORM,
  status: "passed",
  candidateRef: process.env.REHEARSAL_CANDIDATE_REF,
  plannedReleaseTag: process.env.REHEARSAL_PLANNED_TAG,
  commitSha: process.env.REHEARSAL_COMMIT_SHA,
  artifact: {
    name: process.env.REHEARSAL_ARTIFACT_NAME,
    id: Number(process.env.REHEARSAL_ARTIFACT_ID),
    digest: process.env.REHEARSAL_ARTIFACT_DIGEST,
  },
  checks,
};
fs.writeFileSync(process.argv[2], `${JSON.stringify(report, null, 2)}\n`, { flag: "wx" });
NODE
