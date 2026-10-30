# canape

**Control the TV and share the Wi-Fi on any network. No hub, one command, works with Claude.**

canape is a zero-setup remote for the network you're on. Run `canape setup` on a laptop or a Raspberry Pi: it finds the TV and the router, pairs with them, and keeps the Wi-Fi ready to share as a QR code. Then use it from the terminal, your phone or Claude.

There's no hub to install and nothing to keep running, and it works the same in any home you walk into: yours, your family's, a rental.

> Early project. Works today with LG webOS TVs and Freebox routers. Samsung and Sony TVs are next, see [supported devices](#supported-devices).

## Quick start

```bash
brew install lucasnevespereira/tools/canape
canape setup
```

No Homebrew? Download a binary for macOS, Linux or Raspberry Pi from [Releases](https://github.com/lucasnevespereira/canape/releases), or run `go install github.com/lucasnevespereira/canape@latest`.

`canape setup` looks for your TV and router, asks you to accept canape on the TV screen, and saves the Wi-Fi to share. Run it again anytime: it only fills in what's missing.

Then:

```bash
canape                   # what's set up here
canape tv app youtube    # open YouTube on the TV
canape wifi tv           # show the Wi-Fi QR on the TV for your guests
canape serve             # web dashboard
```

`canape help` lists everything.

### Looking around vs setting up

| Command | What it does | Saves |
|---|---|---|
| `canape scan` | Lists every device on the network: TVs, phones, the router, cameras. Use it to look around, or to report a TV canape doesn't support yet. | Nothing |
| `canape setup` | Pairs the TV and router it finds, and saves the Wi-Fi to share. | This network's home |
| `canape devices` | Who's online right now, from the router's own list (Freebox only). | Nothing |
| `canape` | What's set up on this network, and what to do next. | Nothing |

On a new network, start with `canape setup`. Run `canape scan` when you're curious, or when setup finds no TV it knows.

## What it does

- **TV:** on and off, volume, inputs, apps, remote keys, web pages, notifications. `canape tv type` types into the TV, which beats the remote for long logins.
- **Wi-Fi QR code:** in the terminal, on the TV, or on your phone. Guests scan it and join. canape reads the network name from the router and you type the password once. It never changes your router's settings.
- **Network:** `canape scan` lists every device on the network, `canape devices` shows who's online (Freebox).
- **Web dashboard:** the TV remote, inputs, apps and the Wi-Fi QR in any browser, made for your phone.
- **Claude:** an MCP server, so you can ask "put YouTube on the TV" or "who's on the network?".
- **Every network you visit:** your home, your sister's. canape keeps one setup per network and picks the right one by itself.

## Not a home hub

Home Assistant is a hub: installed once per house, always on, thousands of integrations and automations. canape is the other way around: one command on any network, nothing running in the background, a few devices done well. They get along: Home Assistant for the lights and automations at home, canape for the TV and the Wi-Fi wherever you are.

## Supported devices

| Device | Status |
|---|---|
| LG webOS TVs | Supported. Tested on a 2019 LG (webOS 4). [Setup](docs/lg.md) |
| Samsung TVs (Tizen) | Detected by `canape scan`. Driver wanted: [adding a TV](docs/adding-a-tv.md) |
| Sony Bravia TVs | Detected by `canape scan`. Driver wanted |
| Freebox routers | Supported: device list and Wi-Fi name. Tested on a Pop. [Setup](docs/freebox.md) |
| Other routers | Everything works except `canape devices` |

One TV setting matters: on LG, turn on **"Turn on via Wi-Fi"**, or canape can turn the TV off but not back on. See [docs/lg.md](docs/lg.md).

## Web dashboard

```bash
canape serve
```

canape prints a QR code. Scan it with your phone camera and the dashboard opens, already logged in. The link is printed under it too, for a laptop. It logs a device in once, so only use it on your own devices.

On a phone, three tabs sit at the bottom: **Remote** (power, volume, arrows and OK, Home, Back, typing, inputs, apps), **Wi-Fi** (the QR for your guests, and "Show on the TV") and **Network** (who's online). On a computer, all three show side by side, and the keyboard works like the remote: arrows, Enter for OK, Esc for Back, + and − for volume, M to mute.

To use it like an app, add it to your home screen (Share → Add to Home Screen on iPhone). It opens full screen with its own icon.

The dashboard only runs while `canape serve` does. Most things work without it: `canape wifi tv` puts the Wi-Fi QR on the TV in one command.

## Ask Claude

`canape mcp` is an MCP server. Claude starts it when needed, so nothing has to keep running.

```bash
claude mcp add --scope user canape -- canape mcp
```

For Claude Desktop, add this to its config (Settings → Developer → Edit Config). Use the full path that `which canape` prints:

```json
{ "mcpServers": { "canape": { "command": "/opt/homebrew/bin/canape", "args": ["mcp"] } } }
```

Then ask: "put YouTube on the TV", "volume 15", "show the Wi-Fi QR on the TV", "who's on the network?".

Tools: `tv_status`, `tv_power`, `tv_volume`, `tv_mute`, `tv_input`, `tv_launch_app`, `tv_open_url`, `tv_press_key`, `tv_type_text`, `tv_notify`, `wifi_show_on_tv`, `network_devices`. Claude never sees your Wi-Fi password: `wifi_show_on_tv` only puts the QR on the TV.

## Several homes

At someone else's place, join their Wi-Fi and run `canape setup`. canape notices the new network, asks what to call it, and saves it next to yours. Back home, every command uses your home's setup again.

```bash
canape homes                  # list them, → marks where you are
canape homes rename sister
canape homes forget sister
```

canape recognizes a network by its router's MAC address.

## Where it runs

On any machine on the same network as the TV: a Mac, a Linux box, a Raspberry Pi. The TV only takes commands from the local network, so a cloud server can't drive it. For access from outside, put [Tailscale](https://tailscale.com) on that machine and your phone.

## Your data

Everything stays in one file: `~/.canape/state.json`, on every system. It holds the pairing keys and the Wi-Fi passwords, so canape writes it readable by you only. Nothing is sent anywhere else. To start over, delete `~/.canape`. To keep it elsewhere (Docker, tests), set `CANAPE_DATA` to a folder.

## More

- [docs/lg.md](docs/lg.md): LG setup, fewer ads on the home screen, a Wi-Fi app for the TV, how canape talks to the TV
- [docs/freebox.md](docs/freebox.md): Freebox pairing and permissions
- [docs/discovery.md](docs/discovery.md): finding devices by hand
- [docs/adding-a-tv.md](docs/adding-a-tv.md): add support for another TV brand
- [ROADMAP.md](ROADMAP.md): what's next
- [CONTRIBUTING.md](CONTRIBUTING.md)

LG pairing uses the manifest from [PyWebOSTV](https://github.com/supersaiyanmode/PyWebOSTV) (MIT). Thanks.

## License

[MIT](LICENSE)
