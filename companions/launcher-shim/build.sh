#!/usr/bin/env bash
# Cross-builds the launcher shim for every platform it ships for, into ./dist -
# mirrors the root project's own build.sh in spirit (one script, no flags to
# remember), just for this standalone module instead of the whole app.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

rm -rf dist
mkdir -p dist

echo "building linux/amd64 (dowser)..."
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/dowser-linux-amd64 .

echo "building windows/amd64 (dowser.exe) - unverified against real Windows/Steam, see launch_windows.go..."
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/dowser-windows-amd64.exe .

echo "done: $(ls dist)"
