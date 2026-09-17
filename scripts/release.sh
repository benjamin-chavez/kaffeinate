#!/usr/bin/env bash
set -euo pipefail

if [[ $(uname -s) != Darwin ]]; then
  echo "The macOS release must be built on macOS." >&2
  exit 1
fi

release_dir=${1:-dist}
image_name=Kaffeinate-macOS-universal.dmg
mkdir -p "$release_dir"
bash scripts/disk-image.sh "$release_dir/$image_name"
staging_dir=$(mktemp -d "$release_dir/.release.XXXXXX")
trap 'rm -rf "$staging_dir"' EXIT

app_bundle="$staging_dir/Kaffeinate.app"
app_executable="$app_bundle/Contents/MacOS/Kaffeinate"
cli_executable="$app_bundle/Contents/Resources/bin/kaffeinate"
mkdir -p "$app_bundle/Contents/MacOS" "$app_bundle/Contents/Resources/bin"
cp packaging/macos/Info.plist "$app_bundle/Contents/Info.plist"
export MACOSX_DEPLOYMENT_TARGET
MACOSX_DEPLOYMENT_TARGET=$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$app_bundle/Contents/Info.plist")
# Explicit cgo flags include the deployment target in Go's build-cache key.
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET"
export CGO_LDFLAGS="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET"

for architecture in arm64 amd64; do
  # Separate directories avoid executable-name collisions on case-insensitive disks.
  mkdir -p "$staging_dir/$architecture/app" "$staging_dir/$architecture/cli"
  GOOS=darwin GOARCH="$architecture" CGO_ENABLED=1 go build -trimpath \
    -o "$staging_dir/$architecture/app/Kaffeinate" ./cmd/kaffeinate-app
  GOOS=darwin GOARCH="$architecture" CGO_ENABLED=0 go build -trimpath \
    -o "$staging_dir/$architecture/cli/kaffeinate" ./cmd/kaffeinate
done

GOOS=darwin GOARCH="$(go env GOHOSTARCH)" CGO_ENABLED=0 go run ./tools/icons \
  "$app_bundle/Contents/Resources/Kaffeinate.icns"

/usr/bin/lipo -create "$staging_dir/arm64/app/Kaffeinate" "$staging_dir/amd64/app/Kaffeinate" -output "$app_executable"
/usr/bin/lipo -create "$staging_dir/arm64/cli/kaffeinate" "$staging_dir/amd64/cli/kaffeinate" -output "$cli_executable"
chmod 755 "$app_executable" "$cli_executable"

# Merging architecture slices invalidates their existing signatures.
/usr/bin/codesign --force --sign - --timestamp=none "$cli_executable"
/usr/bin/codesign --force --sign - --timestamp=none "$app_bundle"

/usr/bin/lipo "$app_executable" -verify_arch arm64 x86_64
/usr/bin/lipo "$cli_executable" -verify_arch arm64 x86_64
/usr/bin/codesign --verify --strict "$cli_executable"
/usr/bin/codesign --verify --deep --strict "$app_bundle"
/usr/bin/plutil -lint "$app_bundle/Contents/Info.plist"

bash scripts/create-dmg.sh "$app_bundle" "$staging_dir/$image_name"
image_digest=$(/usr/bin/shasum -a 256 "$staging_dir/$image_name")
printf '%s  %s\n' "${image_digest%% *}" "$image_name" >"$staging_dir/SHA256SUMS.txt"

bash scripts/disk-image.sh "$release_dir/$image_name"
mv -f "$staging_dir/$image_name" "$release_dir/$image_name"
mv -f "$staging_dir/SHA256SUMS.txt" "$release_dir/SHA256SUMS.txt"
echo "Built $release_dir/$image_name and $release_dir/SHA256SUMS.txt"
