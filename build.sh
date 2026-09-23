#!/usr/bin/env bash
# Production build with the webkit2gtk-4.1 tag this machine needs (see CLAUDE.md). A release build
# has no developer tools and no Debug tab: those belong to `wails dev` (F5 in VS Code).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
wails build -tags webkit2_41 "$@"

# Each companion under companions/ is built and placed right next to the main app's
# own output (build/bin/companions/<name>-<goos>-<goarch>[.exe]) - internal/launchershim
# resolves them from exactly there at runtime (see its own shimSourcePath), not via
# go:embed, since a companion is a separate Go module the main app never imports.
echo "building companions..."
mkdir -p build/bin/companions
for dir in companions/*/; do
    name=$(basename "$dir")
    (
        cd "$dir"
        GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "../../build/bin/companions/${name}-linux-amd64" .
        GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "../../build/bin/companions/${name}-windows-amd64.exe" .
    )
done
echo "companions built: $(ls build/bin/companions)"
