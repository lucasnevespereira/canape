# canape

A zero-setup remote for the network you're on: the TV, the Wi-Fi and the devices, from the terminal, a phone or Claude. One Go binary; runs on a laptop or a Pi on the same LAN as the devices. Open source, MIT.

## Direction

Tagline: *Control the TV and share the Wi-Fi on any network. No hub, one command, works with Claude.*

What sets canape apart, in the order users care: no hub and one command (`canape setup`), three ways to use it (terminal, phone, Claude), and it works in any home you walk into.

**The filter for every feature:** it belongs in canape if it works in one command on a network canape just joined, without a server that has to run all the time. The dashboard is fine because it's optional.

- In: more TV brands, Chromecast, Apple TV; "what's this device?", new-device alerts; waking a computer; router speed and status; named scenes ("movie mode"); handing lights and sensors to an existing Home Assistant.
- Out: automations ("when X, do Y"), history and graphs, a plugin marketplace, anything that needs a 24/7 server, writing router settings. That's Home Assistant's job.

Supported today: LG webOS TVs, Freebox routers. Protocol notes live in `docs/` (lg.md, freebox.md, discovery.md); read the one for the device you touch before changing its code.

## Decisions and why

- **Runs inside the network, never on a VPS.** TVs only take commands from the LAN, and Wake-on-LAN is a LAN broadcast. Remote access is Tailscale's job.
- **One home per network, keyed by the router's MAC.** `home.Load()` reads the default gateway's MAC from the ARP table (no permission needed; recent macOS hides the Wi-Fi name from CLI tools) and returns that network's home. A new network gets an in-memory home, saved only once something is paired or `canape setup` runs, so read commands never add entries. Nothing ever overwrites another home. A pre-homes `state.json` migrates into a home named "home".
- **TV brands behind one interface.** `tv.TV` is what canape does with any TV. A brand is a driver package under `internal/tv/` plus one entry in `internal/home/drivers.go`. Unsupported actions return `tv.ErrUnsupported`; Wake-on-LAN is shared (`tv.Wake`). Brand-only extras stay in the driver (`lg.TV.Call`, the LG Developer Mode app). Routers aren't abstracted: only the Freebox exists. How to add a brand: `docs/adding-a-tv.md`.
- **Fixed Wi-Fi QR, never router writes.** canape reads the SSID from the router, the user types the password once, it lives only in `state.json`. Generating Freebox guest keys would need the "settings" permission, which can change any router setting; the user chose not to grant it.
- **Stdlib first.** Dependencies only where the stdlib has nothing: `coder/websocket` (LG; it sends no `Origin` header, which LG TVs reject), `rsc.io/qr`, `x/net/dns/dnsmessage` (mDNS), the MCP go-sdk. Each new one needs a reason.
- **One binary, page embedded.** No frontend build. `internal/wifi/page.html` uses old CSS only: webOS 4's browser is an old Chromium (no `min()`, no flex `gap`).
- **Dashboard login by link.** `canape serve` prints a URL with a random key; opening it once moves the key into an HttpOnly, SameSite=Strict cookie. `/wifi` is public: the TV's browser has no cookie, and anyone who can reach it is already on the Wi-Fi.

## Layout

```
main.go                    calls cli.Run, nothing else
internal/cli/              commands and terminal output; setup.go is the guided flow
internal/home/             the current network's home: Load/Save, PairTV/ConnectTV/WakeTV via drivers.go, Status, Devices
internal/store/            state.json: homes keyed by router MAC, dashboard key, legacy migration
internal/discovery/        network scan (SSDP, mDNS, ARP, reverse DNS, control ports), Router(), MAC(), LocalIP(), FindFreebox()
internal/tv/               tv.TV interface, shared types, key names, Wake-on-LAN, name matching (Match)
internal/tv/lg/            LG driver; app.go installs a web app through Developer Mode (wraps LG's ares CLI)
internal/router/freebox/   Freebox OS API client
internal/wifi/             Wi-Fi QR payload and drawings (SVG, terminal, PNG), the big-QR page, ShowOnTV
internal/server/           `canape serve`: login, JSON API under /api, index.html
internal/mcp/              `canape mcp`: MCP tools over stdio (go-sdk imported as sdk)
```

The CLI, the dashboard and the MCP tools all go through `home`. A new capability goes into `tv.TV` (or `home`) first, then gets a CLI command, then a dashboard control and an MCP tool if it fits.

## Rules

- **Under `canape mcp`, stdout is the protocol.** Shared code must not print to stdout: return strings, or write to stderr.
- `state.json` holds pairing keys and Wi-Fi passwords: mode 0600, never log its contents. Its path: `CANAPE_DATA`, else the OS config dir.
- No real IPs, MACs, SSIDs or keys in code, tests or docs.
- Ask the user to run `canape scan` instead of asking them for device details.
- Commits: one-line Conventional Commits.

## Plan

Done, tested on the dev setup (LG 55SM8200 on webOS 4, Freebox Pop): LG control, phone dashboard, Freebox pairing and devices, Wi-Fi QR (terminal, `/wifi`, `canape wifi tv`, LG app), typing on the TV, MCP tools, `internal/` layout, homes, TV interface, `canape setup` and status.

Next:
- [ ] `canape setup` on a second real network (the new-home path is only simulated so far).
- [ ] MCP used from Claude for real (only driven over raw stdio so far).
- [ ] Samsung driver (Tizen WebSocket on 8002, token pairing) and Sony (IP control, pre-shared key), built in front of the real TVs. Scan's detection rules for both are unverified.
- [ ] Releases: prebuilt binaries, Homebrew tap, install script; `canape serve` as a service (launchd, systemd); Docker with host networking.
- [ ] Login QR printed by `canape serve`.
- Later, maybe: Android TV via ADB (XGIMI, Freebox Player), Apple TV, Freebox connection stats, auto-extending LG Developer Mode.

## Out of scope

What fails the filter in Direction, plus: cloud servers or relays, tracking what people browse, rooting TVs.
