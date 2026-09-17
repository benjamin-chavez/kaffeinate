#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

for format_command in git gofmt shfmt; do
  if ! command -v "$format_command" >/dev/null 2>&1; then
    echo "Required command is missing: $format_command. See README.md for formatting setup." >&2
    exit 1
  fi
done

git ls-files -z --cached --others --exclude-standard -- '*.go' '*.sh' |
  while IFS= read -r -d '' source_file; do
    if [[ ! -f "$source_file" || -L "$source_file" ]]; then
      continue
    fi
    case "$source_file" in
    *.go) gofmt -w "./$source_file" ;;
    *.sh) shfmt -i 2 -w "./$source_file" ;;
    esac
  done
