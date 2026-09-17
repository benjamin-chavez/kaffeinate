#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
app_bundle=${1:-dist/Kaffeinate.app}
install_directory=${2:-$HOME/Applications}
bin_directory=${3:-$HOME/.local/bin}

case "$install_directory" in
/*) ;;
*)
  echo "INSTALL_DIR must be an absolute path." >&2
  exit 1
  ;;
esac
case "$bin_directory" in
/*) ;;
*)
  echo "BIN_DIR must be an absolute path." >&2
  exit 1
  ;;
esac

installed_app_bundle="$install_directory/Kaffeinate.app"
if [[ -e "$installed_app_bundle" || -L "$installed_app_bundle" ]]; then
  if [[ -L "$installed_app_bundle" ]] || [[ $(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$installed_app_bundle/Contents/Info.plist") != local.kaffeinate.app ]]; then
    echo "The destination is not a Kaffeinate application bundle." >&2
    exit 1
  fi
fi
if [[ -e "$bin_directory/kaffeinate" && ! -L "$bin_directory/kaffeinate" ]]; then
  echo "A non-symlink kaffeinate command already exists in BIN_DIR." >&2
  exit 1
fi

mkdir -p "$install_directory" "$bin_directory"
staging_directory=$(mktemp -d "$install_directory/.kaffeinate-install.XXXXXX")
trap 'rm -rf "$staging_directory"' EXIT
cp -R "$app_bundle" "$staging_directory/Kaffeinate.app"
rm -rf "$installed_app_bundle"
mv "$staging_directory/Kaffeinate.app" "$installed_app_bundle"
ln -sfn "$installed_app_bundle/Contents/Resources/bin/kaffeinate" "$bin_directory/kaffeinate"
echo "Installed $installed_app_bundle and $bin_directory/kaffeinate"
