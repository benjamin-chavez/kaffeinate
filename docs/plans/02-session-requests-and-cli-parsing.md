# Phase 2: Session requests and CLI compatibility

Prerequisite: Complete [Phase 1](01-native-app-foundation.md).

## Deliverable

A platform-neutral session request model and a side-effect-free CLI parser that understands the agreed `caffeinate` syntax. The controller and native adapter will use the same validated request semantics.

## Compatibility decisions

Support the documented flags and PID-watching forms. The menu requests idle-sleep prevention by default; the CLI can request any of the supported power behaviors explicitly.

| Input | Meaning |
| --- | --- |
| No assertion flags | Request idle-system-sleep prevention. |
| `-i` | Prevent idle system sleep. |
| `-d` | Prevent display sleep. |
| `-m` | Prevent disk idle sleep. |
| `-s` | Prevent system sleep while running on AC power. |
| `-u` | Declare user activity, with the native five-second default when no positive timeout is supplied. |
| `-t seconds` | Supply a nonnegative timeout in seconds; zero supplies no explicit session deadline. |
| `-w pid` | End the request when the specified existing process exits. |
| `-h` | Print help and exit successfully without contacting the app. |

Reject invalid numeric values, missing values, unknown options, and wrapped commands with a nonzero exit code and a useful message. A zero timeout still retains native behavior such as the default `-u` assertion lifetime.

## Implementation checklist

- [x] Inspect the installed `caffeinate` manual and establish isolated native baselines for `-u`, `-u -t 0`, `-u -t 10`, `-iu`, and `-mu`. Record assertion lifetimes separately from process lifetime.
- [x] Record the supported semantics of explicit timeouts, combined flags, and PID watching before writing the request model's tests.
- [x] Define `SessionRequest` using intention-revealing fields for requested power behaviors, an optional positive duration, and an optional watched PID.
- [x] Keep OS command names, subprocess handles, and `systray` types out of the shared request model.
- [x] Define validation that can run both in the CLI and in the application when receiving a request.
- [x] Define the sleep-prevention interface in `internal/power/inhibitor.go`. Acquisition returns an owned handle with release and unexpected-termination behavior.
- [x] Include the timing information the native adapter needs to preserve explicit `-u` timeouts and other native assertion lifetimes.
- [x] Define how effective assertion lifetimes are reported so the controller can distinguish an expired user-activity assertion from other still-active behavior.
- [x] Define a platform process-watching interface that can report an observed PID's exit without exposing macOS mechanisms to the controller.
- [x] Implement argument parsing in `internal/cli/parse.go` using the standard library.
- [x] Accept separate flags, combined boolean flags such as `-di`, and attached values such as `-t28000` and `-w12345` where native short-option syntax permits them.
- [x] Match native option accumulation and repeated-value behavior: assertion flags accumulate, and the last supplied timeout or PID value wins.
- [x] Validate duration conversion for overflow and validate PIDs as positive values within the platform's supported range.
- [x] Handle the `--` option terminator and reject a trailing utility or positional argument with an explanation of the supported scope.
- [x] Implement help output with flag meanings, background behavior, replacement behavior, and representative examples.
- [x] Ensure parsing and help have no application-launch, socket, or power-management side effects.

## Behavioral verification checklist

- [x] Test that a bare command requests indefinite idle-sleep prevention.
- [x] Test that `kaffeinate -i -t 28000` requests idle-sleep prevention for exactly 28,000 seconds.
- [x] Test that combined and separate forms produce equivalent requested behavior.
- [x] Test that explicit flags preserve their meanings without silently adding idle-sleep prevention to every request.
- [x] Test the recorded default and explicit-timeout behavior for user-activity assertions, including combinations with longer-lived assertions.
- [x] Test zero timeout, repeated options, attached values, option termination, and combined timeout/PID requests.
- [x] Test that invalid, missing, negative, and overflowing values fail with useful diagnostics.
- [x] Test that help succeeds and wrapped commands fail without starting or changing a session.

Use concise names such as `when flags are combined, preserves both behaviors` under `TestCLIParse`, and `when the timeout overflows, reports invalid input` under `TestCLIValidation`.

## Acceptance checklist

- [x] The agreed command forms have unambiguous request semantics and behavioral coverage.
- [x] Native user-activity lifetime behavior is understood and recorded rather than inferred from process liveness.
- [x] The shared request model and interfaces can support a Linux adapter without referring to `caffeinate` arguments.
- [x] The focused tests and `go vet ./...` pass.

## Completion evidence

Native baseline on macOS 26.5.2, observed using owned processes and `pmset -g assertions` at 1, 6, and 11 seconds:

- `-u` and `-u -t 0` release user activity after five seconds while their native processes remain alive.
- `-u -t 10` retains user activity past five seconds and exits at ten seconds.
- `-iu` releases user activity after five seconds while idle-system-sleep prevention remains held.
- `-mu` releases both user activity and disk-idle prevention after five seconds. This native combination inherits the user-activity timeout for the later-created disk assertion.

The macOS adapter will report these individual assertion lifetimes. The shared controller will consume those lifetimes without encoding macOS flag-order behavior.

Implemented as `session.Request`, `power.Options`, and owned `power.Hold`/`power.Watch` interfaces. `go test -race ./internal/cli ./internal/session ./internal/power` and `go vet ./...` passed. Parser tests cover the agreed command syntax and errors; native tests verify assertion lifetimes; controller tests verify their user-visible effect.

## Next phase

Continue with [Phase 3: macOS sleep prevention](03-macos-sleep-prevention.md).
