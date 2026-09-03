#!/usr/bin/env bash
set -euo pipefail

# Tauri release packaging needs both GTK/WebKit development headers.  Xvfb
# provides a deterministic display for the packaged runtime smoke on CI.
sudo apt-get update
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
