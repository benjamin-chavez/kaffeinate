#!/usr/bin/env bash
set -euo pipefail

fail_release() {
  echo "$*" >&2
  exit 1
}

require_stopped_app() {
  if app_process_ids=$(pgrep -x Kaffeinate); then
    fail_release "Quit Kaffeinate from its menu before running release checks. Running PID(s): $app_process_ids"
  else
    process_check_status=$?
    if [[ $process_check_status -ne 1 ]]; then
      fail_release "Could not check for running Kaffeinate processes."
    fi
  fi
}

require_clean_checkout() {
  if [[ -n $(git status --porcelain) ]]; then
    fail_release "Commit and merge your changes before creating a release draft."
  fi
}

require_unused_release_tag() {
  release_tags=$(gh api --paginate "repos/$github_repository/releases" --jq '.[].tag_name')
  matching_references=$(gh api "repos/$github_repository/git/matching-refs/tags/$release_tag" --jq '.[].ref')
  if grep -Fxq "$release_tag" <<<"$release_tags" || grep -Fxq "refs/tags/$release_tag" <<<"$matching_references"; then
    fail_release "$release_tag already has a release or tag. Update CFBundleShortVersionString and CFBundleVersion in packaging/macos/Info.plist for the next release."
  fi
}

release_action=${1:-check}
release_directory=${2:-dist}
if [[ $# -gt 2 || ("$release_action" != check && "$release_action" != draft) ]]; then
  fail_release "Usage: bash scripts/release-workflow.sh [check|draft] [release-directory]"
fi
if [[ $(uname -s) != Darwin ]]; then
  fail_release "The macOS release must be built and checked on macOS."
fi

cd "$(dirname "$0")/.."
require_stopped_app

if [[ "$release_action" == draft ]]; then
  require_clean_checkout
  release_commit=$(git rev-parse HEAD)
  release_version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' packaging/macos/Info.plist)
  if [[ ! "$release_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    fail_release "CFBundleShortVersionString must contain a version such as 0.1.1."
  fi
  release_tag="v$release_version"
  github_repository=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
  main_commit=$(gh api "repos/$github_repository/commits/main" --jq .sha)
  if [[ "$release_commit" != "$main_commit" ]]; then
    fail_release "Check out the latest merged main commit from $github_repository before creating a release draft."
  fi
  require_unused_release_tag
fi

verification_directory=$(mktemp -d "${TMPDIR:-/tmp}/kaffeinate release.XXXXXX")
trap 'rm -rf "$verification_directory"' EXIT
unset KAFFEINATE_TEST_RELEASE_DIR KAFFEINATE_TEST_APP_BUNDLE

echo "Building the universal macOS app."
bash scripts/release.sh "$release_directory"
release_directory=$(cd "$release_directory" && pwd)
archive_path="$release_directory/Kaffeinate-macOS-universal.zip"
checksum_path="$release_directory/SHA256SUMS.txt"

echo "Running tests and go vet."
bash scripts/test.sh -count=1
bash scripts/vet.sh

echo "Checking the archive and extracted app."
ditto -x -k "$archive_path" "$verification_directory"
require_stopped_app
KAFFEINATE_TEST_RELEASE_DIR="$release_directory" \
  KAFFEINATE_TEST_APP_BUNDLE="$verification_directory/Kaffeinate.app" \
  go test -race -count=1 -v ./integration 2>&1 | tee "$verification_directory/integration.log"
if grep -q '^--- SKIP:' "$verification_directory/integration.log"; then
  fail_release "A required integration check was skipped. Resolve the reported reason before creating a release draft."
fi
echo "Release checks passed for $archive_path."

if [[ "$release_action" == draft ]]; then
  require_clean_checkout
  if [[ $(git rev-parse HEAD) != "$release_commit" ]]; then
    fail_release "The checked-out commit changed during verification. Run the release workflow again."
  fi
  require_unused_release_tag
  gh release create "$release_tag" "$archive_path" "$checksum_path" \
    --repo "$github_repository" --target "$release_commit" --draft \
    --title "Kaffeinate $release_tag" --generate-notes \
    --notes "Universal macOS app for Apple Silicon and Intel. No Go required. This build lacks Developer ID signing and notarization; see the README for Open Anyway instructions."
  printf '\nReview the draft:\ngh release view %q --repo %q --web\n' "$release_tag" "$github_repository"
  printf '\nPublish when ready:\ngh release edit %q --repo %q --draft=false --latest\n' "$release_tag" "$github_repository"
fi
