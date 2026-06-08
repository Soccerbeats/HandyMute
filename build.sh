#!/usr/bin/env bash
# Cross-compile handymute for Windows (amd64). Run from the repo root.
# Produces dist/handymute.exe (silent) and dist/handymute-console.exe (logs to console).
set -euo pipefail

# Use a local Go toolchain if one was installed to ~/.goroot, else whatever is on PATH.
if [ -x "$HOME/.goroot/bin/go" ]; then
    export GOROOT="$HOME/.goroot"
    export PATH="$HOME/.goroot/bin:$PATH"
fi

export GOOS=windows GOARCH=amd64
mkdir -p dist

echo "Building console build (dist/handymute-console.exe)..."
go build -trimpath -o dist/handymute-console.exe ./cmd/handymute

echo "Building silent build (dist/handymute.exe)..."
go build -trimpath -ldflags="-H windowsgui -s -w" -o dist/handymute.exe ./cmd/handymute

echo "Done:"
ls -la dist/*.exe
