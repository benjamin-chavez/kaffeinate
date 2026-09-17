#!/usr/bin/env bash
set -euo pipefail

if [[ $(uname -s) != Darwin ]]; then
  echo "The application currently builds on macOS." >&2
  exit 1
fi

cd "$(dirname "$0")/.."
app_bundle=${1:-dist/Kaffeinate.app}
rm -rf "$app_bundle"
mkdir -p "$app_bundle/Contents/MacOS" "$app_bundle/Contents/Resources/bin"
CGO_ENABLED=1 go build -trimpath -o "$app_bundle/Contents/MacOS/Kaffeinate" ./cmd/kaffeinate-app
CGO_ENABLED=0 go build -trimpath -o "$app_bundle/Contents/Resources/bin/kaffeinate" ./cmd/kaffeinate
cp packaging/macos/Info.plist "$app_bundle/Contents/Info.plist"
go run ./tools/icons "$app_bundle/Contents/Resources/Kaffeinate.icns"
plutil -lint "$app_bundle/Contents/Info.plist"
