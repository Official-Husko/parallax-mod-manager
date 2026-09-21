#!/usr/bin/env bash
# Production build with the webkit2gtk-4.1 tag this machine needs (see CLAUDE.md), and with
# the web inspector included (-devtools): the Debug tab in Settings turns it on for the person
# using the app; without this flag that switch has nothing to control.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
wails build -tags webkit2_41 -devtools "$@"
