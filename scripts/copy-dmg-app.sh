#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 || $(uname -s) != Darwin ]]; then
  echo "Usage on macOS: bash scripts/copy-dmg-app.sh IMAGE_DMG DESTINATION_APP" >&2
  exit 1
fi
image_path=$1
destination_app=$2
if [[ -e "$destination_app" || -L "$destination_app" ]]; then
  echo "The verification destination already exists: $destination_app" >&2
  exit 1
fi
script_directory=$(cd "$(dirname "$0")" && pwd -P)
source "$script_directory/disk-image.sh"
disk_image_init

[[ $(hdiutil imageinfo -format "$image_path") == UDZO ]] || {
  echo "Expected a compressed, read-only UDZO image." >&2
  exit 1
}
hdiutil verify "$image_path"
disk_image_attach "$image_path" readonly
if [[ ! -d "$disk_image_mountpoint/Kaffeinate.app" || -L "$disk_image_mountpoint/Kaffeinate.app" ||
  ! -L "$disk_image_mountpoint/Applications" || $(readlink "$disk_image_mountpoint/Applications") != /Applications ||
  ! -s "$disk_image_mountpoint/.DS_Store" || ! -f "$disk_image_mountpoint/.background/background.png" ]]; then
  echo "The image is missing its app, Applications shortcut, or saved Finder layout." >&2
  exit 1
fi
sips -g format "$disk_image_mountpoint/.background/background.png" | grep -q 'format: png'
shopt -s dotglob nullglob
for image_entry in "$disk_image_mountpoint"/*; do
  case "$(basename "$image_entry")" in
  Kaffeinate.app | Applications | .background | .DS_Store | .fseventsd | .Trashes | .Spotlight-V100) ;;
  *)
    echo "Unexpected installer content: $image_entry" >&2
    exit 1
    ;;
  esac
done
ditto "$disk_image_mountpoint/Kaffeinate.app" "$destination_app"
disk_image_detach
