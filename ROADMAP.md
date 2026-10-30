# Roadmap

Where canape is going, in order. Every item passes the filter in [AGENTS.md](AGENTS.md#direction): it works in one command on a network canape just joined, with nothing that has to keep running.

## v0.1: first public release

- [x] LG webOS TVs: power, volume, inputs, apps, remote keys, typing, web pages, notifications
- [x] Freebox: pairing and the device list
- [x] Wi-Fi QR code in the terminal, on the TV and in the phone dashboard
- [x] Phone dashboard (`canape serve`)
- [x] Claude through MCP (`canape mcp`)
- [x] Homes: one setup per network, picked automatically
- [x] `canape setup`, the status screen, help grouped by task
- [x] A moved TV is found again, setup names TVs it can't control yet
- [x] `canape version`, releases with binaries, Homebrew
- [x] Make the repo public and tag v0.1.0

Still to check on real hardware before or right after the release:

- `canape setup` on a second network (only simulated so far)
- The MCP tools from Claude itself (only driven by raw test messages so far)
- The automatic Homebrew update on the next release (it failed on v0.1.0, see CONTRIBUTING.md)

## Next

- **More TVs.** Samsung (Tizen) and Sony Bravia, each built in front of a real TV. `canape scan` already recognizes them.
- **Android TV**, through ADB or the Android TV remote protocol. Covers projectors like XGIMI and the Freebox Player.
- **Login QR.** `canape serve` prints the dashboard link as a QR code to scan with the phone.
- **Run in the background.** `canape serve` as a service (launchd on macOS, systemd on Linux and Pi).

## Later

- "What's this device?" in plain words, and an alert when a new device joins
- Wake a computer on the network
- Router speed and connection status (Freebox)
- Named scenes, like "movie mode": TV on, Apple TV input, volume 15
- Hand lights and sensors to Home Assistant when a home already runs it
- Chromecast and Apple TV
- Docker image, for NAS and home servers

## Not planned

Automations, history and graphs, a plugin marketplace, anything that needs a server running all the time, writing router settings. That's Home Assistant's job. See [AGENTS.md](AGENTS.md#direction).
