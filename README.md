# canape

Control your home from the couch. canape is a small self-hosted control center: one Go binary runs on a machine at home and serves a dashboard you open on your phone or laptop.

It starts with an **LG webOS TV** and a **Freebox** router. More devices may come later.

> Status: early. The network scanner works, and the TV commands are built but pairing hasn't been tested on a real TV yet. There's no dashboard yet. The plan is in [AGENTS.md](AGENTS.md#plan).

## What it will do

- **TV:** power on/off, volume, switch inputs (HDMI, Apple TV), launch apps, open a web page on the TV, show on-screen notifications
- **Guest Wi-Fi:** create a temporary Freebox guest key and show it as a QR code on your phone or on the TV. Guests scan it and join. The key expires on its own, and your real password never appears on screen.
- **Network:** see which devices are online right now
- **Ask Claude:** later, an MCP interface so you can say "show guest wifi on the TV" or "turn everything off"

## Where it runs

canape has to run **inside your home network**. The TV only accepts commands from the local network, and turning it on (Wake-on-LAN) only works from there too. A cloud server or VPS can't reach either.

Good options:

- **Raspberry Pi Zero 2 W** (about €20): enough for canape alone
- **Raspberry Pi 4 or 5**: if you also want AdGuard Home or other services
- **Your Mac**: fine for development, but the dashboard goes down when the Mac sleeps

A Freebox Delta or Ultra can run a small Linux VM, so canape could live on the box itself. The Pop can't.

For access from outside your home, install [Tailscale](https://tailscale.com) on the machine and on your phone. There are no ports to open on the router.

## Prepare your devices

### LG TV

1. **Find its IP and MAC address** with `make scan` (or `go run ./cmd/scan`), which lists every device on your network ([how it works](docs/discovery.md)). Give it a fixed IP (a DHCP reservation) on the Freebox so the address never changes.
2. **Enable "Turn on via Wi-Fi"** (sometimes called "Mobile TV On"). Without it, canape can turn the TV off but never back on. The menu path depends on your webOS version. Look under General → Devices or General → External Devices.
3. **Pair once.** The first time canape connects, the TV shows a prompt. Accept it with the remote, and canape keeps the key.

### Freebox

1. **Authorize canape once.** On first start, canape asks the Freebox for access, and you approve the request on the box.
2. **Grant permissions** in Freebox OS (http://mafreebox.freebox.fr) → Paramètres de la Freebox → Gestion des accès → Applications. Creating guest Wi-Fi keys needs "Modification des réglages de la Freebox".

## Run it

The TV part works from the terminal today. The dashboard comes next.

```bash
go install .                         # or prefix each command with: go run .
canape tv pair                       # finds the TV; accept the prompt on screen
canape tv toast "hello from the couch"
canape tv volume 15
canape tv off
canape tv on
canape tv call audio/getVolume       # any SSAP request, prints the reply
```

Run `canape` with no arguments for the full list.

State (TV key, and later the Freebox token and dashboard password) lives in `state.json` in your user config folder (`~/Library/Application Support/canape` on macOS, `~/.config/canape` on Linux), or wherever `CANAPE_DATA` points. Keep it private.

Later, the dashboard will run with `canape` on a Pi or with `docker compose up -d` (host networking), served at `http://<machine-ip>:8080`.

## Fewer ads on the LG home screen

You don't need canape for this. On the TV:

1. Settings → General → System (or Home Settings): turn off **Home Promotion**, **Content Recommendation** and **Home Auto Launch**.
2. Settings → General → System → **User Agreements**: decline viewing information, voice information and interest-based ads.

Menu names change between webOS versions. This removes the large banner on the home screen and most suggested content.

To go further, run [AdGuard Home](https://github.com/AdguardTeam/AdGuardHome) as your network's DNS. It shows every domain each device contacts, so you can see how often the TV phones home and block it. Add blocks gradually: some LG domains also serve the app store and firmware updates.

Rooting the TV ([webOS Homebrew](https://www.webosbrew.org)) gives full control, but only works on older, unpatched firmware. canape doesn't need it.

## Other devices at home

These aren't supported yet, but here is what's possible:

| Device | How it can be controlled |
|---|---|
| Apple TV | Local protocol, see [pyatv](https://pyatv.dev). Play/pause, launch apps, now playing. |
| Android TV (e.g. a XGIMI projector) | ADB over Wi-Fi. To remove ads and bloatware, see [CleanYourTV](https://cleanyour.tv). Keep the vendor apps that handle keystone and focus. |
| Alexa | Unofficial APIs only (alexa-remote-control). They break when Amazon changes its login flow. |

## Ideas for later

- A "Guest Wi-Fi" app tile in the LG app bar that opens the QR full screen. It needs LG Developer Mode, and the developer session must be renewed periodically.
- Scenes, like "movie mode": TV on, Apple TV input, Netflix, volume 20
- Notifications on the TV from other tools: build finished, someone at the door

## Contributing

Design decisions, protocol notes and the build plan are in [AGENTS.md](AGENTS.md). Coding agents (Claude Code, Codex, etc.) read the same file.
