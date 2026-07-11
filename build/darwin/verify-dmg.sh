#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: verify-dmg.sh <JFTrade.dmg>" >&2
  exit 2
fi

dmg_path="$1"
mount_dir="$(mktemp -d "${TMPDIR:-/tmp}/jftrade-dmg-verify.XXXXXX")"
strings_file="${mount_dir}.strings"
mounted=0

cleanup() {
  if [[ "$mounted" -eq 1 ]]; then
    hdiutil detach "$mount_dir" -force >/dev/null 2>&1 || true
  fi
  rm -rf "$mount_dir"
  rm -f "$strings_file"
}
trap cleanup EXIT

hdiutil attach -readonly -noautoopen -mountpoint "$mount_dir" "$dmg_path" >/dev/null
mounted=1

test -d "$mount_dir/JFTrade.app"
test -L "$mount_dir/Applications"
test "$(readlink "$mount_dir/Applications")" = "/Applications"
test -s "$mount_dir/.background/background.png"
test -s "$mount_dir/.DS_Store"
test "$(sips -g pixelWidth "$mount_dir/.background/background.png" | awk '/pixelWidth/ { print $2 }')" = "1320"
test "$(sips -g pixelHeight "$mount_dir/.background/background.png" | awk '/pixelHeight/ { print $2 }')" = "800"
test "$(sips -g dpiWidth "$mount_dir/.background/background.png" | awk '/dpiWidth/ { print int($2) }')" = "144"
test "$(sips -g dpiHeight "$mount_dir/.background/background.png" | awk '/dpiHeight/ { print int($2) }')" = "144"
strings "$mount_dir/.DS_Store" > "$strings_file"
grep -q "background.png" "$strings_file"
grep -q "JFTrade" "$strings_file"

hdiutil detach "$mount_dir" >/dev/null
mounted=0
echo "DMG drag-install layout verified at $dmg_path"
