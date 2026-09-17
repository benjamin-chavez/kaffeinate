#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 || $(uname -s) != Darwin ]]; then
  echo "Usage on macOS: bash scripts/create-dmg.sh APP_BUNDLE OUTPUT_DMG" >&2
  exit 1
fi

script_directory=$(cd "$(dirname "$0")" && pwd -P)
app_bundle=$(cd "$1" && pwd -P)
output_directory=$(cd "$(dirname "$2")" && pwd -P)
output_image="$output_directory/$(basename "$2")"
asset_directory="$script_directory/../packaging/macos"
source "$script_directory/disk-image.sh"
disk_image_init
disk_image_require_unattached "$output_image"

content_directory="$disk_image_workspace/contents"
mkdir -m 755 "$content_directory" "$content_directory/.background"
ditto "$app_bundle" "$content_directory/Kaffeinate.app"
ln -s /Applications "$content_directory/Applications"
cp "$asset_directory/dmg-background.png" "$content_directory/.background/background.png"
staged_size=$(du -A -k -s "$content_directory")
staged_kib=${staged_size%%[[:space:]]*}
image_size_mib=$((staged_kib * 5 / 4 / 1024 + 32))
working_image="$disk_image_workspace/working.dmg"
hdiutil create -srcfolder "$content_directory" -fs HFS+ -volname Kaffeinate \
  -size "${image_size_mib}m" -format UDRW "$working_image"
disk_image_attach "$working_image" readwrite

if ! osascript "$asset_directory/dmg-layout.applescript" "$disk_image_mountpoint"; then
  echo "Could not arrange the installer. Use a logged-in desktop and allow Finder automation in System Settings > Privacy & Security > Automation." >&2
  exit 1
fi
for layout_attempt in {1..20}; do
  [[ ! -s "$disk_image_mountpoint/.DS_Store" ]] || break
  sleep 1
done
if [[ ! -s "$disk_image_mountpoint/.DS_Store" ]]; then
  echo "Finder did not save the installer layout." >&2
  exit 1
fi
disk_image_detach
hdiutil convert "$working_image" -format UDZO -o "$disk_image_workspace/installer.dmg"
hdiutil verify "$disk_image_workspace/installer.dmg"
disk_image_require_unattached "$output_image"
mv "$disk_image_workspace/installer.dmg" "$output_image"
