# macOS releases

Releases contain one universal `Kaffeinate.app` for Apple Silicon and Intel Macs, including its precompiled CLI. Users need no Go installation or developer tools.

## Automated release preparation

Quit Kaffeinate from its menu, then run:

```sh
make release-check
```

This builds the universal app, runs the tests and `go vet`, extracts the ZIP into a temporary path containing spaces, and runs the archive and packaged-app checks. It stops on errors or skipped top-level integration checks and removes the temporary extraction afterward. Architecture checks can still skip when the host cannot run that architecture; review those messages before distributing the app.

To also upload a draft release, install and authenticate the GitHub CLI, update both version fields in `packaging/macos/Info.plist`, and merge those changes. From a clean checkout of the latest `main` commit on GitHub, run:

```sh
make release-draft
```

The command runs the same build and checks, reads the version from `CFBundleShortVersionString`, and uploads the ZIP and checksum to a new draft with generated release notes. It refuses an existing release or tag for that version. It prints the commands to review and publish the draft; publication remains a separate step.

Both commands accept `RELEASE_DIR="/absolute/path with spaces"`. Build output remains gitignored; uploaded files are GitHub Release attachments, outside the repository's tracked files and commit history. `make release` still only builds the ZIP and checksum. The steps below are available for running the process manually.

## Build

Use macOS with Go 1.25+ and Apple Command Line Tools. Build from the intended release commit with a clean working tree.

```sh
make release
```

The command creates:

```text
dist/Kaffeinate-macOS-universal.zip
dist/SHA256SUMS.txt
```

Use `make release RELEASE_DIR="/absolute/path with spaces"` to change the output directory. Every build uses fresh staging, so existing ZIP entries cannot survive a rebuild. Artwork is generated for the build host even when cross-compiling.

The script reads the minimum macOS version from `packaging/macos/Info.plist`. Before a new version, update both `CFBundleShortVersionString` and `CFBundleVersion` in that file and use the matching release tag.

The release uses ad-hoc binary signatures after combining architectures. These preserve executable integrity but provide no Apple Developer ID identity or notarization. The README explains the per-app Open Anyway step; installation does not require disabling Gatekeeper globally.

## Verify

```sh
go test -race ./...
go vet ./...
KAFFEINATE_TEST_RELEASE_DIR="$PWD/dist" go test -race -v ./integration -run '^TestRelease'
```

The release checks validate the checksum, archive layout, executable permissions, universal architecture slices, bundle metadata, local signatures, and CLI help under each available architecture. Intel execution on Apple Silicon requires Rosetta to already be installed. Unavailable architecture checks are reported as skipped.

Quit Kaffeinate, then test the extracted app from a path containing spaces:

```sh
verification_dir=$(mktemp -d "$TMPDIR/kaffeinate release.XXXXXX")
ditto -x -k dist/Kaffeinate-macOS-universal.zip "$verification_dir"
KAFFEINATE_TEST_APP_BUNDLE="$verification_dir/Kaffeinate.app" go test -race -v ./integration
```

Keep the verification directory until the tests finish. The packaged tests skip if another Kaffeinate instance is already running. The enabled suite also runs the app under each available architecture, checking a timed CLI request and cleanup on Quit. Record Intel tests run through Rosetta separately from tests on physical Intel hardware.

The initial release checks passed on Apple Silicon with macOS 26.5.2 and Go 1.26.0, including Intel execution through Rosetta. Both executable slices advertise a minimum version no newer than the bundle's macOS 12 requirement. Runtime checks on macOS 12 and physical Intel hardware have not been performed.

## Publish after the PR is merged

1. Check out the merged commit, rebuild, and complete the checks above.
2. Confirm that the bundle version matches the intended tag and that the tag does not already identify another release.
3. Create a draft with the verified artifacts attached:

   ```sh
   gh release create v0.1.0 \
     dist/Kaffeinate-macOS-universal.zip dist/SHA256SUMS.txt \
     --target "$(git rev-parse HEAD)" --draft --title "Kaffeinate v0.1.0" \
     --notes "Universal macOS app for Apple Silicon and Intel. No developer tools required. This build is unsigned and unnotarized; see the README for Open Anyway instructions."
   ```

4. Check the draft's commit, version, notes, and uploaded assets, then publish it:

   ```sh
   gh release edit v0.1.0 --draft=false --latest
   ```

5. Download the published ZIP and checksum into a fresh directory and compare the checksum. Verify that the README's latest-download link resolves to this asset.
6. Remove the README's first-release-pending sentence once the download is public.

Retain the asset name `Kaffeinate-macOS-universal.zip` in later releases so the README's download link stays valid.
