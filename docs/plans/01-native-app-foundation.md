# Phase 1: Native application foundation

Prerequisite: Read the [plan index](README.md) and its agreed behavior and testing standard.

## Deliverable

A buildable macOS application bundle with an inactive menu bar icon and Quit action, plus a separate CLI executable entry point. This phase establishes that the selected native library works with the local toolchain.

## Implementation checklist

- [x] Confirm the installed Go version, macOS version, machine architecture, and Apple Command Line Tools location.
- [x] Initialize `go.mod` and add `github.com/getlantern/systray` at `v1.2.2`. Record transitive dependencies in `go.sum`.
- [x] Verify that the pinned release compiles with the installed Go and macOS toolchain. Resolve a demonstrated compatibility blocker before building application behavior on it, and document any required version change.
- [x] Add `cmd/kaffeinate-app/main.go` as the native application entry point and `cmd/kaffeinate/main.go` as the CLI entry point.
- [x] Keep `systray` imports in the tray/application path so CLI startup does not initialize the native UI.
- [x] Add a minimal tray module that creates a visible icon, an inactive status item, and a working Quit action.
- [x] Run the native event loop on the macOS main thread according to the library's contract.
- [x] Add `packaging/macos/Info.plist` with the application name, executable name, bundle identifier, and `LSUIElement` configuration.
- [x] Add `make build` to create `dist/Kaffeinate.app` for the current Mac architecture with `Contents/MacOS/Kaffeinate` as the application executable.
- [x] Include the CLI executable at `Contents/Resources/bin/kaffeinate` so it can later locate the containing application bundle.
- [x] Exclude generated build output from version control while retaining source assets and packaging inputs.

## Verification checklist

- [x] Build the application and CLI successfully from the project root.
- [x] Open the bundle with Finder or `open dist/Kaffeinate.app` and verify that its menu bar icon appears without a Dock icon.
- [x] Verify that opening this initial application creates no sleep-prevention request.
- [x] Verify that Quit removes the icon and terminates the application.
- [x] Verify that starting the CLI executable does not create a tray icon until an actual start command is implemented in later phases.
- [x] Run `go vet ./...` for the code introduced in this phase.

## Acceptance checklist

- [x] The native-library compatibility question is resolved with a successful build and launch.
- [x] The two executable entry points are separate and have clear responsibilities.
- [x] `make build` produces the intended bundle layout without requiring manually assembled files.
- [x] The application launches inactive and exits cleanly.

## Completion evidence

Verified on macOS 26.5.2, Apple Silicon, Go 1.26.0, with Command Line Tools at `/Library/Developer/CommandLineTools`.

`go mod tidy`, `make build`, and `go vet ./...` passed with systray v1.2.2. The CLI builds with cgo disabled. Launch Services opened the bundle, System Events reported `background only = true`, and a normal application Quit event terminated it. The initial application has no power-acquisition code.

Automated menu inspection was blocked by macOS Accessibility permissions. The user subsequently confirmed the icon, absence of a Dock icon, menu controls, and Quit manually. No Accessibility permission is needed to run Kaffeinate itself.

## Next phase

Continue with [Phase 2: Session requests and CLI compatibility](02-session-requests-and-cli-parsing.md).
