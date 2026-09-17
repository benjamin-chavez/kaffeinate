# macOS releases

Releases contain a drag-to-Applications disk image with one universal `Kaffeinate.app` for Apple Silicon and Intel Macs, including its precompiled CLI. Users need no Go installation or developer tools. The published `v0.1.0` ZIP remains available; subsequent releases use a DMG.

## Automated release preparation

Quit Kaffeinate from its menu, then run:

```sh
make release-check
```

This builds the universal app and DMG, runs the tests and `go vet`, verifies the checksum and image contents, and copies the app into a temporary path containing spaces. It detaches the image before running packaged-app checks. It stops on errors or skipped top-level integration checks and removes the temporary copy afterward. Architecture checks can still skip when the host cannot run that architecture; review those messages before distributing the app.

To also upload a draft release, install and authenticate the GitHub CLI, update both version fields in `packaging/macos/Info.plist`, and merge those changes. From a clean checkout of the latest `main` commit on GitHub, run:

```sh
make release-draft
```

The command runs the same build and checks, reads the version from `CFBundleShortVersionString`, and uploads the DMG and checksum to a new draft with generated release notes. It refuses an existing release or tag for that version. It prints the commands to review and publish the draft; publication remains a separate step.

Both commands accept `RELEASE_DIR="/absolute/path with spaces"`. Build output remains gitignored; uploaded files are GitHub Release attachments, outside the repository's tracked files and commit history. `make release` only builds the DMG and checksum. The steps below are available for running the process manually.

## Build

Use macOS with Go 1.25+, Apple Command Line Tools, and a logged-in desktop with Finder running. The invoking terminal or application must be allowed to control Finder under **System Settings → Privacy & Security → Automation**. The build briefly opens a Finder window to save the installer layout. Build from the intended release commit with a clean working tree.

```sh
make release
```

The command creates:

```text
dist/Kaffeinate-macOS-universal.dmg
dist/SHA256SUMS.txt
```

Use `make release RELEASE_DIR="/absolute/path with spaces"` to change the output directory. Every build uses fresh staging and verifies a compressed, read-only image before replacing the output. The checksum lists only the DMG; old files in a reused release directory are not uploaded. App artwork is generated for the build host even when cross-compiling.

The installer background is checked in alongside its editable SVG under `packaging/macos/`. After editing the SVG, regenerate the PNG with:

```sh
sips -s format png packaging/macos/dmg-background.svg --out packaging/macos/dmg-background.png
```

Window dimensions and icon positions live in `packaging/macos/dmg-layout.applescript`. Packaging uses one writable HFS+ filesystem, saves its Finder layout, then detaches and converts it directly to UDZO. If an image cannot be detached safely, the command fails and prints the retained workspace and device for recovery. Eject that image before removing the retained workspace or repeating verification.

The script reads the minimum macOS version from `packaging/macos/Info.plist`. Before a new version, update both `CFBundleShortVersionString` and `CFBundleVersion` in that file and use the matching release tag.

The release uses ad-hoc binary signatures after combining architectures. These preserve executable integrity but provide no Apple Developer ID identity or notarization. The README explains the per-app Open Anyway step; installation does not require disabling Gatekeeper globally.

## Verify

```sh
go test -race ./...
go vet ./...
KAFFEINATE_TEST_RELEASE_DIR="$PWD/dist" go test -race -v ./integration -run '^TestRelease'
```

The release checks validate the checksum, UDZO format, image integrity, Applications shortcut, background and saved-layout files, executable permissions, universal architecture slices, bundle metadata, local signatures, and CLI help under each available architecture. Intel execution on Apple Silicon requires Rosetta to already be installed. Unavailable architecture checks are reported as skipped. Eject any existing attachment of the release DMG before verification.

Quit Kaffeinate, then test an independent app copy from a path containing spaces:

```sh
verification_dir=$(mktemp -d "$TMPDIR/kaffeinate release.XXXXXX")
bash scripts/copy-dmg-app.sh dist/Kaffeinate-macOS-universal.dmg "$verification_dir/Kaffeinate.app"
KAFFEINATE_TEST_APP_BUNDLE="$verification_dir/Kaffeinate.app" go test -race -v ./integration
```

Keep the verification directory until the tests finish. The copy command detaches before returning successfully. The packaged tests skip if another Kaffeinate instance is already running. The enabled suite also runs the app under each available architecture, checking a timed CLI request and cleanup on Quit. Record Intel tests run through Rosetta separately from tests on physical Intel hardware.

Check the final DMG in Finder after build staging has been removed. Double-click it normally, confirm the background and app/Applications layout, drag the app into Applications, eject the image, and launch the installed copy. Exercise the menu and bundled CLI. Finder normally opens a read-only image's root automatically; window chrome also follows the user's Finder preferences. Saved-layout file checks do not replace this visual check.

When working in a disposable worktree, put `RELEASE_DIR` outside it and retain the DMG, checksum, source commit, and verification notes before removing the worktree. Use `release-check` for PR verification. Test browser-download quarantine and Open Anyway using the actual hosted release candidate during release preparation, and record those checks separately from local installation tests.

The initial release checks passed on Apple Silicon with macOS 26.5.2 and Go 1.26.0, including Intel execution through Rosetta. Both executable slices advertise a minimum version no newer than the bundle's macOS 12 requirement. Runtime checks on macOS 12 and physical Intel hardware have not been performed.

## Publish after the PR is merged

1. Check out the merged commit, rebuild, and complete the checks above.
2. Confirm that the bundle version matches the intended tag and that the tag does not already identify another release.
3. Create a draft with the verified artifacts attached:

   ```sh
   release_version=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' packaging/macos/Info.plist)
   release_tag="v$release_version"
   gh release create "$release_tag" \
     dist/Kaffeinate-macOS-universal.dmg dist/SHA256SUMS.txt \
     --target "$(git rev-parse HEAD)" --draft --title "Kaffeinate $release_tag" \
     --notes "Drag Kaffeinate into Applications to install. Universal macOS app for Apple Silicon and Intel. No developer tools required. This build lacks Developer ID signing and notarization; see the README for Open Anyway instructions."
   ```

4. Check the draft's commit, version, notes, and uploaded assets, then publish it:

   ```sh
   gh release edit "$release_tag" --draft=false --latest
   ```

5. Browser-download the DMG and checksum into a fresh directory, compare the checksum, and verify installation and first-launch approval. Verify that the README's latest-release link resolves to the release containing the DMG.
6. Remove the README's transitional ZIP instructions once the first DMG release is public.

Retain the asset name `Kaffeinate-macOS-universal.dmg` in later releases.
