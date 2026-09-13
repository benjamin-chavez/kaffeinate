# Phase 6: Menu bar experience and installation

Prerequisite: Complete [Phase 5](05-background-cli-and-local-control.md).

## Deliverable

A complete menu bar interface, accurate rendering of menu- and CLI-created sessions, and a reproducible application bundle with documented CLI installation.

## Menu behavior

```text
Kaffeinate: Inactive
--------------------------
Keep Awake
Keep Awake For           > 15 Minutes
                          30 Minutes
                          1 Hour
                          2 Hours
--------------------------
Quit Kaffeinate
```

Use native menu separators in the actual application. The status line changes to an indefinite session or its end time when active. Additional disabled status items can describe CLI-selected behaviors and a watched PID without forcing all details into one long line.

An active Keep Awake item is checked. Clicking it stops the current session. Selecting a menu duration replaces the current session with the menu's default idle-sleep-prevention behavior.

## Implementation checklist

### Menu and state rendering

- [x] Replace the initial menu with the agreed toggle, timed-session submenu, status display, and Quit action.
- [x] Route all menu commands through the same controller used by the CLI.
- [x] Use an appropriate checkable-menu operation from `systray` so shared menu behavior remains suitable for Linux.
- [x] Serialize UI updates and derive the icon, checkmark, labels, and enabled states from the latest controller snapshot.
- [x] Create distinct inactive and active coffee-cup artwork, such as an inactive cup and an active steaming cup.
- [x] Render macOS icons as template images with suitable high-resolution artwork for light and dark menu bars.
- [x] Include reproducible artwork inputs and embed the required tray assets in the application.
- [x] Show an active timed session's end time without continuously displaying a countdown beside the menu bar icon.
- [x] Display CLI-selected behavior accurately, including display prevention, watched PID, and the AC-power condition of a system-sleep request.
- [x] Update displayed effective behaviors when a short-lived user-activity assertion expires but other behavior remains active.
- [x] Show startup, replacement, and unexpected native failures in the menu while keeping recovery actions usable.
- [x] Keep transition rendering and menu actions responsive without blocking the controller's resource cleanup.
- [x] Ensure Quit follows the shared shutdown path and removes the icon after cleanup.

### Packaging and installation

- [x] Finalize bundle metadata, application icon, executable permissions, and the embedded CLI layout established in Phase 1.
- [x] Keep `make build` reproducible and able to replace its generated bundle without retaining obsolete generated files.
- [x] Document and provide a per-user installation path using `~/Applications/Kaffeinate.app` and a CLI link under `~/.local/bin/kaffeinate`.
- [x] If an installation target is provided, make its destination configurable and ensure it installs only this application's generated artifacts.
- [x] Document how to add the CLI directory to `PATH` without silently editing shell configuration.
- [x] Verify that the CLI resolves its installed application bundle through a symlink and does not depend on the repository's working directory.
- [x] Document rebuilding or reinstalling after relocating the bundle and how to remove the application and CLI link.
- [x] Write the project README with prerequisites, build instructions, menu behavior, CLI examples, compatibility scope, and expected power-management behavior.
- [x] Document that Linux support is planned and identify the future adapter, desktop support, native libraries, and packaging work.

## Verification checklist

- [x] Verify inactive launch, indefinite activation, each timer preset, replacement, stopping, and Quit using the packaged application.
- [x] Start a session from the CLI and verify that its mode, deadline, and watched PID are reflected in the menu.
- [x] Stop a CLI-created session from the menu and verify that its native assertions are removed.
- [x] Replace a CLI-created session with a menu timer and verify that the mode returns to idle-sleep prevention with the newly selected deadline.
- [ ] Verify the icons in light and dark appearance and on the available display's scale factor. Record any display configuration not available for verification.
- [x] Verify that failure messages are readable and that a subsequent valid start can recover.
- [x] Exercise the documented build and installation flow using a temporary installation location before using a persistent user installation.
- [x] Verify that the installed CLI works from a separate terminal with a working directory outside the repository.
- [x] Test shared state-to-presentation behavior where it is meaningful; avoid tests that merely mirror static menu construction or native library internals.

## Acceptance checklist

- [x] The menu and CLI give a consistent view of the one active session.
- [x] The icon is legible, appearance-aware, and visibly distinguishes inactive and active states.
- [x] A built application can be installed and invoked without the source tree or development runtime being required by the user.
- [x] The README accurately explains startup, CLI replacement, flags, cleanup, installation, and the first version's compatibility scope.

## Completion evidence

The user confirmed the visible coffee-cup icon without a Dock icon, steam while active, the top status row's end time, timer presets, the toggle, and Quit. A second manual check confirmed CLI display prevention and a watched PID in the menu, replacement by a 15-minute menu session with idle-only behavior and no PID row, stopping, and Quit.

Template images use reproducible Go artwork compiled into the application. The generated app icon was inspected as a PNG preview. The current desktop appearance was confirmed by the user; separate light/dark-theme switching and alternate display scales were not tested, so that verification item remains unchecked.

`go test -race ./internal/tray ./internal/session` verifies mode/deadline/PID presentation, conditional AC behavior, busy controls, failure messages, recovery, and observer notifications. The final review separated click reception and Quit handling from synchronous native rendering.

`make install` succeeded with both installation paths under a temporary `Install Smoke` directory containing spaces. The installed symlink returned help from `/tmp`, and the installed bundle passed the packaged integration suite. The generated deliverable is `dist/Kaffeinate.app`.

## Next phase

Continue with [Phase 7: End-to-end verification and handover](07-end-to-end-verification.md).
