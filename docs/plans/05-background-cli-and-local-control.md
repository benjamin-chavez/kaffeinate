# Phase 5: Background CLI and single-instance control

Prerequisite: Complete [Phase 4](04-session-controller.md).

## Deliverable

The installed CLI can launch or contact the background application, start or replace its session, receive acknowledgement, and exit while the application continues running. All CLI clients and menu actions share one controller instance.

## Request and lifetime contract

The CLI parses and validates arguments before discovering or launching the application. It sends a structured request over a local Unix-domain socket rather than forwarding a shell command.

A successful acknowledgement means the application validated the request, the controller accepted it, and native launch succeeded. The CLI exits after this response. Asynchronous platform failures can still occur later and must update the application's state.

Help and argument errors do not launch the application. Once a request may have reached the controller, an acknowledgement timeout has an uncertain outcome. Report that uncertainty instead of automatically resending a request that could restart a timer.

## Implementation checklist

### Local control module

- [x] Define a small versioned request/response protocol in `internal/control/protocol.go` using standard-library JSON encoding.
- [x] Include a request identifier, validated session request data, success or error information, and the resulting session description in the response.
- [x] Implement a bounded single-request exchange with explicit read/write deadlines and a maximum message size.
- [x] Place the socket and instance coordination files in a per-user runtime location that fits Unix socket path limits.
- [x] Restrict the runtime directory and socket to the owning user, using directory mode `0700` and socket mode `0600` where applicable.
- [x] Acquire an OS-released exclusive instance lock before owning the socket and application state.
- [x] Recover a stale socket only while holding the instance lock, and avoid unlinking another live instance's socket.
- [x] Validate decoded requests again in the application before submitting them to the controller.
- [x] Keep the local control module independent of `systray`, and route accepted requests through the existing controller.
- [x] Reject malformed, oversized, incompatible-version, and incomplete requests without changing the current session. Handle a disconnect after request admission according to the acknowledgement contract.

### Application startup and CLI client

- [x] Wire the application to own one controller and one local control server for its lifetime.
- [x] Ensure simultaneous application launches converge on one instance before creating duplicate tray icons or controllers.
- [x] Preserve an existing session when Finder or Launch Services opens the application again.
- [x] Resolve the CLI's containing application bundle from its executable location, following installation symlinks and handling spaces in paths.
- [x] Attempt to contact the existing instance before launching a new application.
- [x] Launch through `/usr/bin/open -g -n` after a failed connection. The instance lock rejects duplicate owners, and `-n` bypasses stale Launch Services registrations.
- [x] Wait for the control socket using bounded readiness checks and report a useful launch or readiness failure.
- [x] Submit a validated start request and wait for acknowledgement without waiting for the session to end.
- [x] Print a concise confirmation that includes the requested behaviors, deadline or indefinite duration, and watched PID when present.
- [x] Return a nonzero exit code for parse, launch, connection, protocol, or native-start failure. Use exit code `2` for invalid CLI usage and `1` for operational failure.
- [x] Avoid automatically resubmitting a mutating request after an ambiguous connection or acknowledgement failure.
- [x] Coordinate app Quit, normal termination signals, local-server shutdown, and controller cleanup without depending on UI callbacks completing further rendering.

## Behavioral verification checklist

- [x] Test that a stopped application is launched and receives a request successfully.
- [x] Test that a running application receives replacement requests without launching another instance.
- [x] Test that the CLI exits after acknowledgement while the session remains active.
- [x] Test that closing the initiating terminal or its owned test harness does not end the application's session.
- [x] Test that help and invalid arguments neither launch the application nor change an existing session.
- [x] Test that launch failure, readiness timeout, invalid server responses, and native-start failure produce useful errors and nonzero exit codes.
- [x] Test that a lost acknowledgement reports an uncertain outcome without silently restarting the session.
- [x] Test concurrent clients, duplicate application launches, stale sockets, and recovery after an application crash.
- [x] Test malformed messages and early client disconnects through the public socket interface.
- [x] Test the installed or symlinked CLI from a working directory outside the repository and from a bundle path containing spaces.
- [x] Verify that both application quit and abrupt application termination release their native sleep-prevention resources.

Example descriptions include `when acknowledged, exits before the session ends` under `TestCLIStart`, and `when clients launch concurrently, starts one app instance` under `TestAppStartup`.

## Acceptance checklist

- [x] `kaffeinate -i -t 28000` activates the background application and returns the prompt after acknowledgement.
- [x] There is one owning application and one active session across all entry points.
- [x] CLI and terminal lifetimes are independent of the active session.
- [x] Errors are observable without leaking processes, sockets, or watches.
- [x] Focused CLI/control tests pass, including race-enabled tests for concurrent access.

## Completion evidence

The runtime socket is `/tmp/kaffeinate-<uid>/control.sock`. `go test -race ./internal/control ./internal/cli` and `go vet ./...` passed. A real `-i -t 28000` invocation returned in 0.322 seconds, and PID-scoped `pmset` output confirmed only the requested idle-system-sleep assertion.

The second independent review found early lock release during shutdown and acceptance of incomplete success acknowledgements. Shutdown now drains clients, releases the session, and then releases instance ownership. A blocked-release test verifies that a competing instance cannot take ownership during cleanup. Success responses validate their session state. Socket-level regressions cover invalid acknowledgements and verify that a lost response causes exactly one submitted request without moving its original deadline.

Real packaged checks passed with a bundle under a path containing spaces and a symlinked CLI invoked outside the source tree:

```sh
KAFFEINATE_TEST_APP_BUNDLE="/var/folders/2x/dx11zdrj6971tc69m2m4jwzh0000gn/T/opencode/CLI Smoke/Kaffeinate.app" go test -race -v ./integration
```

These checks cover terminating the initiating terminal helper, replacement, invalid requests, reopening the app, both PID/timeout orderings, abrupt application death, restart, and concurrent CLI launches. They exposed Launch Services error `-600` after immediate relaunch. Using `open -g -n` fixed the issue; the focused regression passed three consecutive runs and the original packaged suite passed afterward.

## Next phase

Continue with [Phase 6: Menu bar experience and installation](06-menu-bar-and-installation.md).
