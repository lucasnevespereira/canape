# Roadmap

Where canape is going, in order. Every item passes the filter in [AGENTS.md](AGENTS.md#direction): it works in one command on a network canape just joined, with nothing that has to keep running.

## v0.1: first public release

- [x] LG webOS TVs: power, volume, inputs, apps, remote keys, typing, web pages, notifications
- [x] Freebox: pairing and the device list
- [x] Wi-Fi QR code in the terminal, on the TV and in the web dashboard
- [x] Web dashboard (`canape serve`)
- [x] Login QR: `canape serve` prints the dashboard link as a QR code to scan with the phone
- [x] Dashboard v2: Remote, Wi-Fi and Network tabs on a phone, side by side on a computer, a remote-style dial, keyboard shortcuts, add to home screen
- [x] Quieter dashboard: graphite theme, only 8 apps until "Show all", network folded to one line, Wi-Fi password hidden until "Show"
- [x] Homebrew formula updated automatically on release (v0.2.0)
- [x] Computer remote on macOS: play, pause, skip, volume, full screen on the machine canape runs on (CLI, dashboard, MCP)
- [x] Dashboard organized by device: a page per device, phone tabs built from the devices with More for the rest, sidebar on a computer
- [x] Claude through MCP (`canape mcp`)
- [x] Homes: one setup per network, picked automatically
- [x] `canape setup`, the status screen, help grouped by task
- [x] A moved TV is found again, setup names TVs it can't control yet
- [x] `canape version`, releases with binaries, Homebrew
- [x] Make the repo public and tag v0.1.0

Still to check on real hardware before or right after the release:

- `canape setup` on a second network (only simulated so far)
- The MCP tools from Claude itself (only driven by raw test messages so far)
- The dashboard added to an iPhone home screen: it should log itself in on first launch
- Play/pause with the space bar and Click the video on a web player (the first test with the media key was unreliable)

## Next

- **More TVs.** Samsung (Tizen) and Sony Bravia, each built in front of a real TV. `canape scan` already recognizes them.
- **Android TV**, through ADB or the Android TV remote protocol. Covers projectors like XGIMI and the Freebox Player.
- **Run in the background.** `canape serve` as a service (launchd on macOS, systemd on Linux and Pi).

## Later

- Computer keys on Linux (playerctl, xdotool) and Windows (media keys)

- "What's this device?" in plain words, and an alert when a new device joins
- Wake a computer on the network
- Router speed and connection status (Freebox)
- Named scenes, like "movie mode": TV on, Apple TV input, volume 15
- Hand lights and sensors to Home Assistant when a home already runs it
- Chromecast and Apple TV
- Docker image, for NAS and home servers

## Not planned

Automations, history and graphs, a plugin marketplace, anything that needs a server running all the time, writing router settings. That's Home Assistant's job. See [AGENTS.md](AGENTS.md#direction).
