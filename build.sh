#!/usr/bin/env bash
# Production build with the webkit2gtk-4.1 tag this machine needs (see CLAUDE.md). A release build
# has no developer tools and no Debug tab: those belong to `wails dev` (F5 in VS Code).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
wails build -tags webkit2_41 "$@"
