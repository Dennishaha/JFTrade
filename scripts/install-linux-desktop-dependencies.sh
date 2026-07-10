#!/usr/bin/env bash
set -euo pipefail

# Wails task/doctor uses the default GTK4 stack, while the shipped Linux
# desktop binary is deliberately built with the legacy GTK3 build tag.
sudo apt-get update
sudo apt-get install -y \
  file \
  pkg-config \
  libgtk-4-dev \
  libwebkitgtk-6.0-dev \
  libgtk-3-dev \
  libwebkit2gtk-4.1-dev
