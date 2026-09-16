# Kaffeinate

Keep your Mac awake from the menu bar or terminal. Commands run in the background, so your terminal stays free.

## Install

The download will be available after the first release is published.

1. [Download Kaffeinate for macOS](https://github.com/benjamin-chavez/kaffeinate/releases/latest/download/Kaffeinate-macOS-universal.zip).
2. Extract the ZIP if needed, then move **Kaffeinate.app** into **/Applications**.
3. Open the app. macOS blocks the first launch because this release is unsigned and unnotarized.
4. Open **System Settings → Privacy & Security**, scroll down, and click **Open Anyway**.
5. Open the app again and click **Open**.

Once running, the app shows a coffee cup in the menu bar and no Dock icon. Apple documents the approval steps in [Safely open apps on your Mac](https://support.apple.com/en-us/102445).

The app is built for Apple Silicon and Intel Macs running macOS 12 or newer. It has been tested on macOS 26 with Apple Silicon. No Go or developer tools are needed.

Each release also includes `SHA256SUMS.txt`. To check the download, run this in the folder containing both files:

```sh
shasum -a 256 -c SHA256SUMS.txt
```

## Menu bar

The app opens inactive. Click the coffee cup to control it:

- **Keep Awake** turns sleep prevention on or off.
- **Keep Awake For** offers 15 minutes, 30 minutes, 1 hour, or 2 hours.
- **Quit Kaffeinate** stops the session and closes the app.

Menu sessions let the display sleep. Steam marks an active session. Timed sessions show their end time at the top of the menu.

To open it at login, add Kaffeinate under **System Settings → General → Login Items & Extensions → Open at Login**.

## Terminal

To use the CLI after installing the download, add this to your shell configuration:

```sh
alias kaffeinate='/Applications/Kaffeinate.app/Contents/Resources/bin/kaffeinate'
```

Change the path if the app lives somewhere else. Open a new terminal afterward. The CLI opens the app automatically when needed.

macOS may block the first CLI run the same way it blocked the app. Approve it with **Open Anyway** again, or clear quarantine for the whole app once:

```sh
xattr -dr com.apple.quarantine /Applications/Kaffeinate.app
```

| Command | Behavior |
| --- | --- |
| `kaffeinate` | Prevent idle sleep indefinitely. |
| `kaffeinate -i -t 28000` | Prevent idle sleep for 28,000 seconds. |
| `kaffeinate -di -t 3600` | Keep the Mac and display awake for one hour. |
| `kaffeinate -i -w 12345` | Keep the Mac awake until PID 12345 exits. |
| `kaffeinate -h` | Show all options. |

Starting a new session replaces the current one. Stop it from the menu. Wrapped commands such as `kaffeinate make` are unsupported.

## Build from source

Building requires macOS, Go 1.25+, and Apple Command Line Tools.

```sh
make install
export PATH="$HOME/.local/bin:$PATH"
make test
make vet
```

`make install` builds the app and installs it in `~/Applications`. `make build` only creates `dist/Kaffeinate.app`.

See the [release guide](docs/releases.md) for packaging and publication, or the [implementation plan](docs/plans/README.md) for architecture. Linux support is planned.
