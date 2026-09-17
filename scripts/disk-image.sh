#!/usr/bin/env bash

disk_image_init() {
  disk_image_workspace=$(mktemp -d "${TMPDIR:-/tmp}/kaffeinate image.XXXXXX")
  disk_image_workspace=$(cd "$disk_image_workspace" && pwd -P)
  disk_image_mountpoint="$disk_image_workspace/volume"
  disk_image_pending=false
  disk_image_path=
  trap disk_image_cleanup EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
}

disk_image_locate() {
  local image_count image_index image_path image_directory entity_count entity_index entity_mount
  disk_image_device=
  disk_image_current_mount=
  hdiutil info -plist >"$disk_image_workspace/images.plist" || return 2
  image_count=$(plutil -extract images raw -expect array -o - "$disk_image_workspace/images.plist") || return 2
  for ((image_index = 0; image_index < image_count; image_index++)); do
    image_path=$(plutil -extract "images.$image_index.image-path" raw -o - "$disk_image_workspace/images.plist") || return 2
    if [[ "$image_path" != "$disk_image_path" ]]; then
      [[ "$image_path" == /* ]] || continue
      image_directory=$(cd "$(dirname "$image_path")" 2>/dev/null && pwd -P) || continue
      image_path="$image_directory/$(basename "$image_path")"
    fi
    [[ "$image_path" == "$disk_image_path" ]] || continue
    entity_count=$(plutil -extract "images.$image_index.system-entities" raw -expect array -o - "$disk_image_workspace/images.plist") || return 2
    for ((entity_index = 0; entity_index < entity_count; entity_index++)); do
      if [[ -z "$disk_image_device" ]]; then
        disk_image_device=$(plutil -extract "images.$image_index.system-entities.$entity_index.dev-entry" raw -o - "$disk_image_workspace/images.plist") || return 2
      fi
      if entity_mount=$(plutil -extract "images.$image_index.system-entities.$entity_index.mount-point" raw -o - "$disk_image_workspace/images.plist" 2>/dev/null); then
        disk_image_current_mount=$(cd "$entity_mount" && pwd -P) || return 2
      fi
    done
    [[ "$disk_image_device" == /dev/disk* ]] || return 2
    return 0
  done
  return 1
}

disk_image_require_unattached() {
  local image_path=$1 lookup_status image_directory
  [[ "$disk_image_pending" == false ]] || return 1
  image_directory=$(cd "$(dirname "$image_path")" && pwd -P) || return 1
  disk_image_path="$image_directory/$(basename "$image_path")"
  if disk_image_locate; then
    echo "Eject the already attached image before continuing: $disk_image_path" >&2
    return 1
  else
    lookup_status=$?
    [[ $lookup_status -eq 1 ]] || return "$lookup_status"
  fi
}

disk_image_attach() {
  local image_path=$1 mount_mode=$2
  [[ -f "$image_path" && ! -L "$image_path" ]] || {
    echo "Expected a regular disk image file: $image_path" >&2
    return 1
  }
  disk_image_require_unattached "$image_path" || return 1
  mkdir "$disk_image_mountpoint"
  disk_image_pending=true
  hdiutil attach "$disk_image_path" "-$mount_mode" -nobrowse -noautoopen \
    -mountpoint "$disk_image_mountpoint" -plist >"$disk_image_workspace/attachment.plist" || return 1
  disk_image_locate || return 1
  if [[ "$disk_image_current_mount" != "$disk_image_mountpoint" ]]; then
    echo "The image did not mount at its requested private location." >&2
    return 1
  fi
}

disk_image_detach() {
  local lookup_status detach_attempt
  [[ "$disk_image_pending" == true ]] || return 0
  if disk_image_locate; then
    if [[ -n "$disk_image_current_mount" && "$disk_image_current_mount" != "$disk_image_mountpoint" ]]; then
      echo "The image is mounted elsewhere; leaving that attachment alone." >&2
      return 1
    fi
  else
    lookup_status=$?
    if [[ $lookup_status -eq 1 ]]; then
      disk_image_pending=false
      return 0
    fi
    return "$lookup_status"
  fi
  for detach_attempt in 1 2 3; do
    if hdiutil detach "$disk_image_device"; then
      disk_image_pending=false
      return 0
    fi
    [[ $detach_attempt -eq 3 ]] || sleep 1
  done
  return 1
}

disk_image_cleanup() {
  local exit_status=$?
  trap - EXIT
  if disk_image_detach; then
    rm -rf "$disk_image_workspace"
  else
    echo "Could not safely detach $disk_image_path (${disk_image_device:-unknown device})." >&2
    echo "Retained image workspace for recovery: $disk_image_workspace" >&2
    [[ $exit_status -ne 0 ]] || exit_status=1
  fi
  exit "$exit_status"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  set -euo pipefail
  if [[ $# -ne 1 ]]; then
    echo "Usage: bash scripts/disk-image.sh IMAGE_DMG" >&2
    exit 1
  fi
  disk_image_init
  disk_image_require_unattached "$1"
fi
