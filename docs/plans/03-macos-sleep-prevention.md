# Phase 3: macOS sleep prevention

Prerequisite: Complete [Phase 2](02-session-requests-and-cli-parsing.md).

## Deliverable

A macOS adapter that acquires and releases the requested native power assertions, owns its subprocess resources, and supports watching a user-specified PID. Its behavior is verified independently of the tray UI.

## Lifetime contract

The long-lived menu bar application owns native sleep prevention. Its adapter invokes `/usr/bin/caffeinate` directly without a shell and monitors the application's PID with native `-w` behavior.

A user-supplied `-w` PID is an additional lifetime condition. Observe that PID separately so app termination and watched-process termination can both release the request. Passing only the user's PID to the native command would lose the application's lifetime protection.

The controller owns application deadlines. The native adapter also receives timeout information when needed to preserve native assertion behavior, especially an explicit timeout for `-u`.

## Implementation checklist

- [x] Implement the macOS sleep-prevention adapter behind the interface defined in Phase 2.
- [x] Execute `/usr/bin/caffeinate` with explicit argument values and no shell interpolation.
- [x] Pass only the assertion flags required by the validated request and preserve their macOS meanings.
- [x] Tie the native process to the menu bar application's PID so abrupt application termination releases its request.
- [x] Apply requested native timeout semantics, including explicit user-activity duration and the recorded default behavior of mixed assertion flags.
- [x] Capture useful startup and asynchronous failure information for the controller without retaining terminal file descriptors.
- [x] Distinguish successful process launch, later native failure, expected release, and effective assertion expiration.
- [x] Implement idempotent release that terminates and reaps only the adapter's owned child process.
- [x] Bound graceful cleanup and provide a force-termination fallback for an owned child that does not exit.
- [x] Implement PID watching in `internal/power/watch_darwin.go` using a native exit notification supported by the standard library, such as kqueue.
- [x] Define and implement the already-exited PID behavior: produce a completed watch so the controller does not leave an active request for a process that has already ended.
- [x] Handle watch registration errors and process exit during registration without leaking a watcher or power request.
- [x] Close native watch resources on session replacement, cancellation, and application shutdown.

## Behavioral verification checklist

- [x] Exercise the adapter through its acquisition and release interface using real child processes and bounded waits.
- [x] Verify that requesting idle-sleep prevention creates the corresponding assertion without creating a display-sleep-prevention assertion.
- [x] Verify that a display request creates the expected display assertion and that explicit combined requests retain each requested behavior.
- [x] Verify the recorded default and explicit-timeout behavior of user-activity assertions. Check effective assertions rather than child-process existence alone.
- [x] Verify that release removes the adapter's assertions and reaps its child.
- [x] Verify that repeated release succeeds without affecting unrelated processes.
- [x] Verify that native startup failure and unexpected child exit are observable through the public interface.
- [x] Verify PID exit, an already-exited PID, watch cancellation, and exit during registration using owned helper processes.
- [x] Terminate an owned helper application abruptly and verify that its native sleep-prevention child also exits and releases its assertions.
- [x] Identify assertions by owned process information when using `pmset -g assertions`; leave unrelated system assertions and existing `caffeinate` sessions alone.

Example descriptions include `when released, removes its power assertions` under `TestSleepPrevention` and `when the watched process exits, reports completion` under `TestProcessWatch`.

## Acceptance checklist

- [x] Native power behavior matches the validated request and the Phase 2 baseline.
- [x] App lifetime, watched-process lifetime, and native assertion lifetimes can all be observed correctly.
- [x] All adapter-owned processes and watch resources have a verified cleanup path.
- [x] Focused macOS integration tests pass with reliable cleanup on test failure.

## Completion evidence

`go test -race ./internal/power` passed on macOS 26.5.2. Integration checks use owned processes and PID-scoped `pmset` observations. They cover idle/display assertions, native timeout expiry, default mixed activity expiry, explicit activity lasting beyond five seconds, unexpected native death, repeated release, cancellation, and abrupt owner death.

PID watching uses kqueue with a cancellation pipe and close-on-exec descriptors. Tests cover live process exit, cancellation without killing the watched process, already-reaped PIDs, and exit racing registration. An isolated helper with an exhausted descriptor limit verifies real native-start failure without changing the parent process's limits.

The first independent review requested explicit user-activity timeout coverage and stronger controller shutdown cleanup. Both are implemented and tested.

## Next phase

Continue with [Phase 4: Shared session controller](04-session-controller.md).
