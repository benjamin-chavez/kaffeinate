# Kaffeinate

Kaffeinate is a Go menu bar app for macOS that controls the built-in `caffeinate` utility. Start a session from its coffee-cup menu or your terminal, then get back to work.

```sh
kaffeinate -i -t 28000
```

This keeps your Mac from idle sleeping for 28,000 seconds and returns your terminal prompt after the app acknowledges the session. The app starts in the background if necessary, and closing the terminal does not end the session.

## Build and open

Building requires macOS, Go 1.25 or newer, and Apple Command Line Tools with cgo enabled for the application. The CLI itself builds with cgo disabled. Development has been verified on Apple Silicon with Go 1.26.0 and macOS 26.5.2.

```sh
make build
open dist/Kaffeinate.app
```

The app starts inactive and appears only in the menu bar. It has no Dock icon. Its menu offers:

- **Keep Awake** starts an indefinite session. Clicking it while checked stops the current session.
- **Keep Awake For** starts a session for 15 minutes, 30 minutes, 1 hour, or 2 hours.
- **Quit Kaffeinate** releases the session and exits the app.

Menu-started sessions prevent idle system sleep while allowing the display to sleep. An active cup has steam, and the menu shows the session's behavior, end time, and watched PID when applicable. Selecting a menu duration replaces the current session with idle-system-sleep prevention.

Kaffeinate does not require Accessibility or Screen Recording access. Those permissions are unnecessary for its menu, CLI, and sleep-prevention behavior.

## Install the app and CLI

```sh
make install
```

This installs:

```text
~/Applications/Kaffeinate.app
~/.local/bin/kaffeinate
```

The CLI path is a symlink to the executable inside the app bundle. Add its directory to your shell's `PATH` if needed:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

You can put that line in your shell configuration to retain it across terminal sessions. The installer does not edit your shell configuration.

Installation directories can be changed using absolute paths:

```sh
make install INSTALL_DIR="$HOME/Applications" BIN_DIR="$HOME/.local/bin"
```

Quit the running app before reinstalling a new build. If you move the bundle, update the CLI symlink or run the installer again. All copies use the same per-user application instance.

You can also use the bundled CLI directly before installing:

```sh
dist/Kaffeinate.app/Contents/Resources/bin/kaffeinate -i -t 28000
```

End users need only the app bundle and optional CLI link. Go and development tools are needed to build it, not to run it. The local build is intended for local use; a notarized public release is not part of this build workflow.

## Command-line behavior

```text
kaffeinate [-dimsu] [-t seconds] [-w pid]
```

| Option | Behavior |
| --- | --- |
| No assertion options | Prevent idle system sleep indefinitely. |
| `-i` | Prevent idle system sleep. |
| `-d` | Keep the display awake. |
| `-m` | Prevent disk idle sleep. |
| `-s` | Prevent system sleep while on AC power. The request remains held on battery, with that condition shown in the menu. |
| `-u` | Declare user activity, lasting five seconds by default. |
| `-t seconds` | End the session after this many seconds. Zero supplies no explicit timeout. |
| `-w pid` | End when that existing process exits, or when the timeout arrives first. The process must be accessible to the current user. |
| `-h` | Print help without opening the app. |

Examples:

```sh
kaffeinate
kaffeinate -i -t 28000
kaffeinate -di -t 3600
kaffeinate -i -w 12345
kaffeinate -i -t 3600 -w 12345
kaffeinate -u -t 10
```

Combined flags, attached values such as `-t3600`, and separate values are supported. Repeated assertion flags accumulate. The last timeout or PID value wins. Durations must be nonnegative whole seconds; PIDs must be positive integers.

The CLI accepts flags and PID watching. Wrapped utility execution, such as `kaffeinate make`, is not included in this version. Watch an already-running process with `-w` instead.

If you explicitly watch your shell or a job that exits when its terminal closes, that process's exit will end the session.

### One session, shared by the CLI and menu

Every successful start command replaces the current session with the new flags and duration. Invalid input leaves the existing session alone. If acquiring a valid replacement fails after the previous session is released, the menu shows an inactive session with the error.

CLI success means the application accepted the request and launched native sleep prevention. Later native failures appear in the menu. If the CLI loses its acknowledgement, it reports an uncertain result and does not resubmit the request automatically; check the menu before retrying.

Exit codes are `0` for help or an acknowledged request, `1` for an operational failure, and `2` for invalid usage. Stop an active session using the menu's checked **Keep Awake** item.

### Native lifetime details

The app retains macOS `caffeinate` behavior, including a subtle mixed-flag case: without an explicit positive timeout, `-mu` gives both user activity and disk-idle prevention a five-second lifetime. With `-iu`, user activity expires after five seconds while idle-system-sleep prevention remains active. An explicit positive `-t` supplies the requested duration.

The controller tracks assertion expiry independently of the native process. This avoids showing a session as active merely because `caffeinate` is still running after its assertions expire. Quitting or terminating Kaffeinate releases its own assertions; unrelated `caffeinate` sessions remain independent.

Idle-sleep prevention follows macOS power-management rules. It does not promise to override closing the laptop lid or explicitly putting the machine to sleep.

## Architecture

- `cmd/kaffeinate` is the short-lived CLI.
- `cmd/kaffeinate-app` owns the menu bar application and shutdown.
- `internal/session` owns session state, replacement, and deadlines in one event loop.
- `internal/power` implements macOS assertions and PID watching behind shared interfaces.
- `internal/control` provides bounded local JSON exchanges over a Unix-domain socket and single-instance ownership.
- `internal/tray` renders state and forwards menu actions.
- `internal/artwork` contains the reproducible coffee-cup artwork used for tray and app icons.

The control socket lives at `/tmp/kaffeinate-<uid>/control.sock` inside a user-owned directory with mode `0700`. Its socket has mode `0600`. Instance ownership is retained until session cleanup completes.

The UI library is `github.com/getlantern/systray` v1.2.2. Its transitive dependencies are recorded in `go.mod` and `go.sum`. Application behavior otherwise uses Go's standard library and built-in macOS tools.

Linux support is planned. Shared session logic, request parsing, and control communication are reusable. A Linux power adapter, desktop-specific tray verification, native-library packaging, and a target distribution/desktop decision are still needed. Linux support should report unsupported power behavior explicitly.

## Verify a build

```sh
go test -race ./...
go vet ./...
make build
```

Tests exercise observable behavior through module interfaces. Native power tests use owned helper processes and inspect only their assertions. The default suite does not launch or quit the menu bar app.

To run packaged CLI checks, quit Kaffeinate first and provide an absolute bundle path:

```sh
KAFFEINATE_TEST_APP_BUNDLE="$PWD/dist/Kaffeinate.app" go test -race -v ./integration
```

These tests launch an app, exercise terminal independence, timers, PID watching, replacement, crashes, and concurrent startup, then clean up their instance. They skip when the environment variable is missing or an existing Kaffeinate process is running.

Menu appearance, light/dark rendering, and physical menu clicks have a manual checklist in [the menu phase](docs/plans/06-menu-bar-and-installation.md). The complete implementation record is in [docs/plans](docs/plans/README.md).

## Remove

Quit Kaffeinate, remove `~/Applications/Kaffeinate.app`, and remove its `~/.local/bin/kaffeinate` symlink. Adjust those paths if you used custom installation directories. No login item is installed.
