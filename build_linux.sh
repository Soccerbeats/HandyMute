#!/usr/bin/env bash
# Build handymute for Linux (amd64). Run from the repo root.
# Requires: Go 1.25+, and dev headers for X11/Xtst, GTK3, WebKit2GTK-4.1, ayatana-appindicator3.
#   sudo apt-get install -y libx11-dev libxtst-dev libgtk-3-dev \
#       libwebkit2gtk-4.1-dev libayatana-appindicator3-dev
set -euo pipefail

if [ -x "$HOME/.goroot/bin/go" ]; then
    export GOROOT="$HOME/.goroot"
    export PATH="$HOME/.goroot/bin:$PATH"
fi

mkdir -p dist
echo "Building dist/handymute (linux/amd64)..."
CGO_ENABLED=1 go build -trimpath -o dist/handymute ./cmd/handymute
echo "Done:"
ls -la dist/handymute
