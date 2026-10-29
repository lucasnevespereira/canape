# canape

Control your home from the couch. canape is a small self-hosted control center: one Go binary runs on a machine at home and serves a dashboard you open on your phone or laptop.

It starts with an **LG webOS TV** and a **Freebox** router. More devices may come later.

> Status: early. The network scanner and the TV commands work (tested on a 2019 LG). The first dashboard is in, with TV controls only. The plan is in [AGENTS.md](AGENTS.md#plan).

## What it will do

- **TV:** power on/off, volume, switch inputs (HDMI, Apple TV), launch apps, open a web page on the TV, show on-screen notifications
- **Wi-Fi QR code:** show your Wi-Fi as a QR code on your phone or on the TV. Guests scan it and join. canape reads the network name from the Freebox and never changes your Wi-Fi settings.
- **Network:** see which devices are online right now
- **Ask Claude:** an MCP server, so you can say "put YouTube on the TV", "show the Wi-Fi to my guests" or "who's on the network?"

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

1. **Find its IP and MAC address** with `canape scan`, which lists every device on your network ([how it works](docs/discovery.md)). Give it a fixed IP (a DHCP reservation) on the Freebox so the address never changes.
2. **Enable "Turn on via Wi-Fi"** (sometimes called "Mobile TV On"). Without it, canape can turn the TV off but never back on. The menu path depends on your webOS version. Look under General → Devices or General → External Devices.
3. **Pair once.** The first time canape connects, the TV shows a prompt. Accept it with the remote, and canape keeps the key.

### Freebox

1. **Authorize canape once** with `canape freebox pair`. The box shows the request on its small screen: use its arrows to pick "Oui" and confirm. On a Pop, the screen may stay on "Authentification en cours" afterwards; that's a known display bug and pairing still works.
2. **No extra permission needed.** canape only reads from the box. Leave "Modification des réglages de la Freebox" unticked for canape in Freebox OS (Paramètres de la Freebox → Gestion des accès → Applications).

## Run it

### Dashboard

Pair the TV first (`canape tv pair`, below), then:

```bash
make serve        # or: canape serve [addr], default :8080
```

canape prints a link like `http://192.168.1.10:8080/?key=…`. Open it once on each of your devices: it logs that device in with a cookie and drops the key from the address bar. Only open it on your own devices. To log everyone out, delete the `dashboard` key from `state.json` and restart.

On iPhone, Share → Add to Home Screen makes it feel like an app.

### Terminal

```bash
go install .                         # or prefix each command with: go run .
canape tv pair                       # finds the TV; accept the prompt on screen
canape tv toast "hello from the couch"
canape tv volume 15
canape tv input                      # list inputs; canape tv input hdmi1 switches
canape tv app                        # list apps; canape tv app youtube launches it
canape tv open https://example.com   # opens the TV's browser
canape tv button home                # remote keys: home, back, up, down, left, right, enter
canape tv type                       # type into the focused field on the TV (asks for the text)
canape tv off
canape tv on
canape tv call audio/getVolume       # any SSAP request, prints the reply
canape freebox pair                  # confirm on the Freebox screen
canape freebox devices               # who's online; add "all" for every device seen
canape wifi set                      # save the Wi-Fi password once (name comes from the Freebox)
canape wifi                          # Wi-Fi QR code in the terminal
canape wifi tv                       # Wi-Fi QR on the TV, no server needed
```

Run `canape` with no arguments for the full list.

State (TV key, dashboard login key, Freebox token, Wi-Fi name and password) lives in `state.json` in your user config folder (`~/Library/Application Support/canape` on macOS, `~/.config/canape` on Linux), or wherever `CANAPE_DATA` points. Keep it private.

Later, a Pi install script and `docker compose up -d` (host networking) will run the dashboard permanently.

## Ask Claude

`canape mcp` is an MCP server, so Claude can control the TV and read the network. Nothing has to stay running: Claude starts it when needed.

```bash
go install .
claude mcp add --scope user canape -- "$(go env GOPATH)/bin/canape" mcp
```

For Claude Desktop, add this to its config (Settings → Developer → Edit Config), with your own path:

```json
{ "mcpServers": { "canape": { "command": "/Users/you/go/bin/canape", "args": ["mcp"] } } }
```

Then ask: "put YouTube on the TV", "turn the volume down to 15", "show the Wi-Fi QR on the TV", "who's on the network right now?".

Tools: `tv_status`, `tv_power`, `tv_volume`, `tv_mute`, `tv_input`, `tv_launch_app`, `tv_open_url`, `tv_press_key`, `tv_type_text`, `tv_notify`, `wifi_show_on_tv`, `network_devices`. Claude never sees your Wi-Fi password: `wifi_show_on_tv` only puts the QR on the TV.

## Wi-Fi app on the TV

You probably don't need this: `canape wifi tv` shows the QR on the TV in one command, and the dashboard's "Show on TV" button does the same. The app is for showing it with no computer at all.

It's a "Wi-Fi" tile in the TV's app bar that shows the QR code, even when canape isn't running. It's a small webOS app with your Wi-Fi baked in, installed through LG's Developer Mode.

One-time setup on the TV (LG requires it; canape can't do it for you):

1. Create a free account at [webostv.developer.lge.com](https://webostv.developer.lge.com).
2. On the TV, open the **Developer Mode** app (install it from the LG Content Store if needed), sign in, turn on **Dev Mode Status**, and let the TV restart.
3. Open Developer Mode again and turn on **Key Server**.

Then on your computer:

```bash
npm install -g @webos-tools/cli     # LG's CLI, once
canape tv install-app
```

canape already knows the TV's address, so the only question is the passphrase shown in Developer Mode, and only the first time. Run `canape tv install-app` again whenever you change the Wi-Fi password.

Developer Mode only lasts a limited time. The app shows a countdown: press **Extend** before it runs out, or the TV removes apps installed this way.

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
