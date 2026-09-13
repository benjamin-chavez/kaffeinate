# Kaffeinate implementation plan

This plan delivers a macOS menu bar application and a background command-line client written in Go. It preserves the application logic needed for future Linux support.

Implementation and phase acceptance are complete. Automated checks and the user's menu checks passed. Separate theme-switching and display-scale checks remain explicitly unverified in Phase 6.

## How to use these documents

Work through the phases in order. Each phase names its prerequisite, deliverable, implementation tasks, behavioral checks, and completion criteria.

Mark an item complete only after its work and verification are finished. Record the commands, results, and relevant manual observations in the phase's completion evidence section. Mark a phase complete below only after its acceptance checklist is satisfied.

## Phase checklist

- [x] Complete [Phase 1: Native application foundation](01-native-app-foundation.md).
- [x] Complete [Phase 2: Session requests and CLI compatibility](02-session-requests-and-cli-parsing.md).
- [x] Complete [Phase 3: macOS sleep prevention](03-macos-sleep-prevention.md).
- [x] Complete [Phase 4: Shared session controller](04-session-controller.md).
- [x] Complete [Phase 5: Background CLI and single-instance control](05-background-cli-and-local-control.md).
- [x] Complete [Phase 6: Menu bar experience and installation](06-menu-bar-and-installation.md).
- [x] Complete [Phase 7: End-to-end verification and handover](07-end-to-end-verification.md).

## Agreed product behavior

### Menu bar application

- The application is named Kaffeinate and has a coffee-cup menu bar icon without a Dock icon.
- Opening the application normally starts it inactive. Opening an already running application preserves its current session.
- The menu can start an indefinite session or a session lasting 15 minutes, 30 minutes, 1 hour, or 2 hours.
- Menu-started sessions prevent idle system sleep while allowing the display to sleep.
- Selecting a duration replaces the existing session and starts the selected duration from activation.
- Clicking the active session toggle stops the session. Quitting releases the session and exits the application.
- The icon, checkmark, status, deadline, and any CLI-selected power behavior reflect the controller's current state.

### Background CLI

The following invocation must work:

```sh
kaffeinate -i -t 28000
```

It starts an idle-sleep-prevention session lasting 28,000 seconds, launches the menu bar application if needed, and returns the terminal prompt after the application acknowledges the request. The background application owns the session, so closing the terminal does not stop it.

- A bare `kaffeinate` command starts an indefinite idle-sleep-prevention session.
- Support `-d`, `-i`, `-m`, `-s`, `-u`, `-t`, and `-w`, including combined short flags such as `-di`.
- Preserve documented macOS flag behavior, including the AC-power condition for `-s` and the default five-second user-activity assertion for `-u`.
- A positive timeout and a watched PID end a session at whichever condition occurs first.
- Each successful start request replaces the one existing session. Requests do not toggle an active session off.
- Invalid requests leave the current session unchanged. Help and argument errors do not launch the application.
- Support `-h` for help. Wrapped commands such as `kaffeinate make` are outside this version's compatibility scope and must produce a clear error.
- Acknowledgement confirms that the controller accepted the request and its native launch succeeded. Later platform failures are reflected in the application's state and menu.

### Platform and dependency decisions

- Use Go and `github.com/getlantern/systray`, initially pinned to `v1.2.2`, with its transitive dependencies recorded in `go.sum`.
- Use Go's standard library for parsing, process management, timers, local communication, and tests wherever practical.
- Building the macOS application requires Go, cgo, and Apple Command Line Tools. End users receive an application bundle without a separate Go or package-manager installation.
- Keep shared session logic independent of macOS and of `systray`.
- Add Linux through a platform adapter, platform-specific icon handling, and packaging in a future effort. Its distribution and desktop environment are still to be selected.
- Linux tray support and native library requirements must be verified on the selected desktop. Report unsupported power behavior explicitly when Linux is implemented.
- Launch-at-login, persistence across application restarts, multiple concurrent sessions, and wrapped-command execution are outside the agreed first version.

## Architecture

```text
CLI executable                         Menu bar executable
Parses arguments                       Owns the application lifetime
Launches the app if necessary           Hosts the local control server
        |                                        |
        +------ Unix-domain socket --------------+
                                                 |
                                      Shared session controller
                                      State, deadlines, replacement
                                                 |
                                      Sleep-prevention interface
                                                 |
                                      macOS platform adapter
                                      caffeinate and PID watching
```

Use these patterns at the indicated locations:

1. Deep modules keep process management and session rules behind small interfaces.
2. A finite state machine defines inactive, starting, active, stopping, and closed behavior. The UI receives a coherent public snapshot.
3. A single-owner event loop serializes session commands, expiration, platform events, and shutdown.
4. Platform adapters implement sleep prevention and process watching. The shared controller consumes behavior rather than subprocess details.
5. Constructor-based dependency injection connects modules and permits controlled behavioral tests.
6. Local client-server communication lets the short-lived CLI control the long-lived menu bar application.

The application entry point creates the modules and coordinates shutdown. The controller owns an active sleep-prevention handle. The macOS adapter owns its child processes and native watching resources. UI work must never be required to complete resource cleanup.

An operating-system power assertion requests a change to sleep or user-activity behavior. A process can remain alive after one of its assertions expires, so process liveness alone must not determine active session status.

## Intended source layout

```text
cmd/
  kaffeinate/main.go
  kaffeinate-app/main.go
internal/
  cli/
    parse.go
    run.go
    launch_darwin.go
  control/
    protocol.go
    client.go
    server.go
    instance.go
  session/
    request.go
    controller.go
  power/
    inhibitor.go
    inhibitor_darwin.go
    watch_darwin.go
  tray/
    menu.go
    icons_darwin.go
    assets/
packaging/
  macos/Info.plist
Makefile
go.mod
go.sum
README.md
docs/plans/
```

Place Go tests beside the behavior they verify. File names identify intended responsibilities; add or split files only when the implementation needs it. Linux-specific implementations will be added when Linux work begins.

## Testing standard

Test observable behavior through module interfaces. A refactor that preserves behavior should preserve the tests without rewriting assertions about private fields, helper calls, internal channels, or goroutine counts.

Use a top-level Go test name to identify the operation. Give each `t.Run` case a concise description that states its condition and expected outcome. For example:

| Test group | Case description |
| --- | --- |
| `TestSessionStart` | `when activation fails, remains inactive` |
| `TestSessionStop` | `when active, releases sleep prevention` |
| `TestSessionReplacement` | `when the old deadline passes, keeps the new session active` |
| `TestCLIStart` | `when acknowledged, exits before the session ends` |
| `TestProcessWatch` | `when the watched process exits, ends the session` |

Use controlled dependencies for timing and error conditions. Verify real subprocess behavior in focused macOS integration tests. Use explicit synchronization and bounded waits instead of arbitrary sleeps. Assertions about power behavior must identify this application's resources rather than rely on machine-wide assertion totals.

## Final verification commands

```sh
go test -race ./...
go vet ./...
make build
```

The final phase also verifies the packaged application, installed CLI, native power assertions, and cleanup. No phase is complete solely because it compiles.

## Reference material

- [systray v1.2.2 documentation](https://github.com/getlantern/systray/tree/v1.2.2)
- The installed macOS manual, available with `man caffeinate`.
- [Apple's caffeinate source](https://github.com/apple-oss-distributions/PowerManagement/blob/main/caffeinate/caffeinate.c)
- [XDG desktop portal inhibition interface](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.Inhibit.html)

The installed operating system is the reference for integration checks. Where flag combinations have subtle behavior, record the observed baseline before encoding expectations.
