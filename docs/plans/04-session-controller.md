# Phase 4: Shared session controller

Prerequisite: Complete [Phase 3](03-macos-sleep-prevention.md).

## Deliverable

A platform-neutral controller that owns one session, serializes commands and lifetime events, publishes coherent state, and cleans up without depending on native UI work.

## State and ownership rules

The public state distinguishes inactive sessions from indefinite and timed active sessions. It includes the effective power behaviors, optional watched PID, deadline, and any actionable error. Internal starting, stopping, and closed states coordinate resource transitions.

The controller owns its acquired power handle and process watcher. The platform adapter owns the native resources behind those handles. State notifications carry values that callers can observe without reading mutable implementation fields.

## Implementation checklist

- [x] Create the controller with explicit dependencies for sleep prevention and process watching. Use Go's `testing/synctest` for deterministic standard-library timers.
- [x] Expose a small interface for starting a validated request, stopping, observing current state and updates, and closing the controller.
- [x] Use one event loop to own mutable state and serialize commands, native exits, watched-process exits, deadlines, and shutdown.
- [x] Start inactive and reject new starts after closure.
- [x] Validate a new request before disturbing the current session. Treat validation failure as an unchanged session.
- [x] Replace a valid active session by releasing its resources before acquiring a new request, preserving the single-session ownership rule.
- [x] If replacement acquisition fails after the previous session was released, report inactive state with the failure.
- [x] If release cannot be confirmed, report the error and avoid starting a second request whose ownership would overlap the first.
- [x] Use each acquired handle's completion channel as its session identity. Stop selecting replaced handles so old events cannot affect a new session.
- [x] Own timed-session deadlines and release the request when its duration expires.
- [x] Keep deadlines consistent with the displayed end time. After system resume or delayed event processing, end a session whose deadline has already passed.
- [x] End a session when its watched PID exits, or its explicit duration expires, whichever occurs first.
- [x] Apply the effective assertion lifetimes established in Phase 2. A default user-activity assertion can expire while other requested behavior remains active.
- [x] Return to inactive state when all requested assertions have expired or been released, even if the native subprocess would otherwise stay alive.
- [x] Preserve a held AC-only system-sleep request while the machine is on battery, and expose its conditional behavior. A power-source condition is distinct from assertion expiration.
- [x] Publish state updates without allowing a slow UI or disconnected observer to block commands or cleanup. Preserve access to the latest state.
- [x] Treat expected release and expiration as successful completion while surfacing unexpected platform failures.
- [x] Make Stop and Close idempotent, and finish owned-resource cleanup before Close returns or reports its bounded cleanup error.

## Behavioral verification checklist

- [x] Test indefinite and timed activation through the controller's public interface.
- [x] Test that activation failure leaves the controller inactive with an observable error.
- [x] Test that invalid replacement requests leave the existing session unchanged.
- [x] Test successful replacement, replacement acquisition failure, and release failure without inspecting internal fields.
- [x] Test that a replaced session's old deadline or exit event cannot stop the new session.
- [x] Test that explicit deadlines release power requests, including deadlines already passed when processing resumes.
- [x] Test that watched-process exit and timeout each end the session when they occur first.
- [x] Test default user-activity expiration alone and alongside longer-lived assertions.
- [x] Test that an AC-only request retains its lifetime while exposing the power-source condition.
- [x] Test Stop while active, Stop while inactive, repeated Close, and start attempts after Close.
- [x] Test that shutdown completes even when a state observer has stopped receiving updates.
- [x] Test simultaneous requests and lifetime events for coherent observable outcomes and single-session ownership.
- [x] Run the controller tests under the race detector using controlled time and dependencies rather than arbitrary sleeps.

Use names such as `when already inactive, succeeds` under `TestSessionStop` and `when the old deadline passes, keeps the new session active` under `TestSessionReplacement`.

## Acceptance checklist

- [x] The UI and CLI can use the same controller interface without knowing subprocess details.
- [x] The controller has no imports of `systray` or macOS-only process APIs.
- [x] All stated session transitions and ownership rules have behavioral coverage.
- [x] Expiration, replacement, and shutdown remain correct under race-enabled tests.

## Completion evidence

`go test -race ./internal/session` and `go vet ./...` passed. Controller tests use the same Start, Stop, Current, Changes, and Close interface as application callers. Go's synthetic-time testing verifies deadlines without real-time delays.

The first milestone received independent standards and specification reviews. Fixes include kind-qualified names, independent watcher cleanup when power release fails, preservation of native errors during overdue expiration, and disarming automatic retries after terminal cleanup failure. Each failure path has a behavioral regression test. Explicit wall-clock deadlines are rechecked at most one second after timer processing resumes.

## Next phase

Continue with [Phase 5: Background CLI and single-instance control](05-background-cli-and-local-control.md).
