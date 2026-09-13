# Phase 7: End-to-end verification and handover

Prerequisite: Complete [Phase 6](06-menu-bar-and-installation.md).

## Deliverable

A verified local macOS application and CLI, with behavioral test coverage, documented verification evidence, and a clear handover for future Linux work.

## Automated verification checklist

- [x] Run `go test -race ./...` successfully after completing the application and CLI integration.
- [x] Run `go vet ./...` successfully.
- [x] Run `make build` successfully and verify the expected application and CLI artifacts.
- [x] Exercise short-duration end-to-end sessions so timeout behavior is verified without waiting for the production menu durations.
- [x] Verify help, malformed input, unsupported wrapped commands, and app launch failure through the CLI executable's observable output and exit status.
- [x] Verify start, replacement, stop, deadline expiration, watched-PID expiration, and shutdown through the implemented public interfaces.
- [x] Verify concurrency, stale-instance recovery, disconnected clients, and ambiguous acknowledgements without asserting private synchronization details.
- [x] Verify that native integration tests clean up their own processes and assertions after both success and failure.
- [x] Review test descriptions for a clear condition and expected behavior, using the top-level test name for shared context.
- [x] Remove tests whose assertions merely reproduce implementation details or private helper behavior; retain coverage of the actual outcomes.

## Packaged-application acceptance checklist

- [x] Open `Kaffeinate.app` normally and verify inactive status, one menu bar icon, and no Dock icon.
- [x] Run `kaffeinate -i -t 28000` from the installed CLI and verify prompt return after acknowledgement and an approximately 7-hour, 46-minute, 40-second session in the app. Stop this long session after verification.
- [x] Start another CLI session and verify that it replaces the first session in the same application instance.
- [x] Close the initiating terminal and verify that the active session remains owned by the menu bar application.
- [x] Run a short timed session and verify that expiration updates the menu and removes the application's native sleep-prevention assertion.
- [x] Start a PID-watched session against an owned helper process, terminate that helper, and verify the session ends.
- [x] Verify both timeout-first and PID-exit-first behavior for requests containing both conditions.
- [x] Exercise explicit display prevention and verify that the menu reports the requested mode.
- [x] Verify default and explicit-duration user-activity requests, including a combination with longer-lived power behavior.
- [x] Inspect `pmset -g assertions` for this application's owned assertions and confirm menu-started idle prevention does not add display prevention.
- [x] Verify normal Quit, termination signals, and abrupt application termination release owned assertions and do not terminate unrelated `caffeinate` processes.
- [x] Reopen the application after termination and verify that stale control files do not prevent startup.
- [x] Verify that opening an already running app does not reset or replace its current session.
- [x] Verify that an invalid CLI request leaves the current session unchanged.
- [x] Verify the installed CLI and bundle from paths containing spaces and from a directory outside the source tree.

## Documentation and portability checklist

- [x] Confirm that the README's build, installation, invocation, and removal instructions match the finished artifacts.
- [x] Document the tested macOS version, architecture, Go version, and pinned native-library version.
- [x] Record the flags' actual power-management semantics, including AC-dependent behavior and assertion lifetimes that differ from process lifetime.
- [x] Confirm that shared session logic contains no macOS UI or native process-watching details.
- [x] Confirm that the CLI parser and local protocol describe requested behaviors using shared types rather than raw shell commands.
- [x] Record the remaining Linux choices: target distribution and desktop, supported power APIs and flag mappings, tray availability, native dependencies, packaging, and platform acceptance tests.
- [x] Record unavailable hardware or environmental checks as unverified instead of marking them complete.

## Completion criteria

- [x] All previous phase acceptance checklists are complete with evidence.
- [x] Required automated checks pass and the packaged CLI example works as agreed.
- [x] Active-state rendering and resource cleanup have been verified through real operating-system behavior.
- [x] Remaining work is explicitly documented, with future Linux implementation separated from completed macOS behavior.
- [x] Update the [plan index](README.md) to reflect completed phases and provide the user with the build location, usage instructions, and verification summary.

## Completion evidence

### Automated results

Verified on macOS 26.5.2, Apple Silicon, Go 1.26.0, and systray v1.2.2:

```sh
go test -race ./...
go vet ./...
make build
KAFFEINATE_TEST_APP_BUNDLE="$PWD/dist/Kaffeinate.app" go test -race -v ./integration
```

All commands passed. The default test suite deliberately skips packaged GUI-process checks unless the environment variable is set. The explicitly enabled suite ran without those skips and passed in about 19 seconds. Its focused concurrent-start regression also passed ten consecutive runs after accounting for transient duplicate startup processes exiting without acquiring ownership.

The packaged suite verifies the 28,000-second CLI request through a terminal helper that is then terminated; short deadlines; default, explicit, and mixed user activity; both PID/timeout orderings; reopening; invalid requests; crash recovery; and concurrent launches. A suspended native child forces slow shutdown to verify that an incoming launch waits for the previous owner to finish cleanup.

Failure-cleanup checks deliberately fail after a confirmed native assertion, both after launch and after restart. They require an explicit checkpoint and verify that the owned application is removed. Suspended native children have independent, PID-scoped cleanup. No test uses broad process termination.

Installation and the enabled suite also passed under a temporary `Install Smoke/Applications/Kaffeinate.app` path. The actual installed CLI symlink returned help with `/tmp` as its working directory. The final local deliverable is `dist/Kaffeinate.app`; persistent installation is available with `make install`.

The shared session test package also cross-compiled for Linux/amd64 with cgo disabled. This verifies compilation of the shared logic, not a Linux application implementation.

### Independent review results

Independent specification and standards reviewers examined the three agreed milestones. Addressed findings include watcher cleanup during failed release, overdue native-error reporting, bounded failure retries, instance-lock retention through cleanup, validation of acknowledgements, Launch Services relaunch behavior, menu clicks during rendering, startup during shutdown, observer coverage, and robust test cleanup. The reviewers confirmed the targeted final corrections.

### Manual results and remaining checks

The user confirmed the icon, absence of a Dock icon, active steam, top-row end time, timer presets, toggle, and Quit. A second check confirmed CLI-created display prevention and PID details, replacement by a 15-minute idle-only menu session, removal of the PID row, stopping, and Quit.

Separate light/dark-theme switching and alternate display scales remain unverified and are left unchecked in Phase 6. Future Linux work still requires selecting a distribution and desktop, implementing its power adapter, verifying flag mappings and tray support, and packaging its native dependencies.
