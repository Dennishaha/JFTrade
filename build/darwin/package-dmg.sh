#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: package-dmg.sh <JFTrade.app> <output.dmg>" >&2
  exit 2
fi

app_path="$1"
output_path="$2"
volume_name="JFTrade"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/jftrade-dmg.XXXXXX")"
staging_dir="$work_dir/root"
readwrite_dmg="$work_dir/JFTrade-rw.dmg"
device=""

cleanup() {
  if [[ -n "$device" ]]; then
    hdiutil detach "$device" -force >/dev/null 2>&1 || true
  fi
  rm -rf "$work_dir"
}
trap cleanup EXIT

detach_device() {
  local attempt

  for attempt in 1 2 3 4 5; do
    if hdiutil detach "$device" >/dev/null; then
      device=""
      return 0
    fi
    if [[ "$attempt" -lt 5 ]]; then
      sleep 1
    fi
  done

  echo "disk image is still busy after 5 detach attempts; forcing detach of $device" >&2
  hdiutil detach "$device" -force >/dev/null
  device=""
}

if [[ ! -d "$app_path" ]]; then
  echo "application bundle is missing: $app_path" >&2
  exit 1
fi

if mount | grep -q " on /Volumes/$volume_name "; then
  echo "/Volumes/$volume_name is already mounted; eject it before packaging" >&2
  exit 1
fi

mkdir -p "$staging_dir/.background" "$(dirname "$output_path")"
cp -R "$app_path" "$staging_dir/JFTrade.app"
ln -s /Applications "$staging_dir/Applications"
mkdir -p "$staging_dir/.fseventsd"
touch "$staging_dir/.fseventsd/no_log" "$staging_dir/.metadata_never_index"
sips -s format png "$root_dir/build/darwin/dmg-background.svg" \
  --out "$staging_dir/.background/background.png" >/dev/null
sips -s dpiWidth 144 -s dpiHeight 144 \
  "$staging_dir/.background/background.png" >/dev/null

hdiutil create -volname "$volume_name" -srcfolder "$staging_dir" \
  -ov -format UDRW "$readwrite_dmg" >/dev/null
device="$(hdiutil attach -readwrite -noverify -noautoopen \
  "$readwrite_dmg" | awk 'NR == 1 { print $1 }')"

osascript <<'APPLESCRIPT'
tell application "Finder"
  delay 1
  tell disk "JFTrade"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set pathbar visible of container window to false
    set sidebar width of container window to 0
    set the bounds of container window to {100, 100, 760, 500}
    set viewOptions to the icon view options of container window
    set arrangement of viewOptions to not arranged
    set icon size of viewOptions to 112
    set text size of viewOptions to 13
    set background picture of viewOptions to file ".background:background.png"
    set position of item "JFTrade.app" of container window to {170, 210}
    set position of item "Applications" of container window to {490, 210}
    update without registering applications
    delay 2
    close
  end tell
end tell
APPLESCRIPT

sync
detach_device
rm -f "$output_path"
hdiutil convert "$readwrite_dmg" -format UDZO -imagekey zlib-level=9 \
  -o "$output_path" >/dev/null

echo "DMG written to $output_path"
