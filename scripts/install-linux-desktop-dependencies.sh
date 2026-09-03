#!/usr/bin/env bash
set -euo pipefail

# Tauri release packaging needs both GTK/WebKit development headers.  Xvfb
# provides a deterministic display for the packaged runtime smoke on CI.
sudo apt-get update

if [[ "${1:-}" == "--runtime-only" ]]; then
  sudo apt-get install -y --no-install-recommends \
    file \
    libwebkit2gtk-4.1-0 \
    libgtk-3-0 \
    libayatana-appindicator3-1 \
    xvfb
  exit 0
fi

sudo apt-get install -y \
  clang \
  file \
  libappindicator3-dev \
  xvfb \
  libgtk-3-dev \
  libwebkit2gtk-4.1-dev \
  librsvg2-dev \
  mold \
  patchelf \
  pkg-config

