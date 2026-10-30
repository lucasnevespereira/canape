# canape

A helper for any Wi-Fi you join: it tells a non-expert what's on the network and how good it is, and controls the devices there (TVs, the computer it runs on), from the terminal, a phone or Claude. One Go binary on a laptop or a Pi. Open source, MIT.

## Direction

Tagline: *Join any Wi-Fi, understand it, and control what's on it.*

The picture: you arrive at a family member's house, join their Wi-Fi, type `canape`, and within seconds you know the network (Wi-Fi quality, the box, whether the internet works, every device in plain words) and can control what's there.

Who it's for: someone who has never heard of an IP address. Plain words, every problem with its next step, no terminal for everyday use, read-only on the router. The principles are at the top of ROADMAP.md.

- **Works without the router.** On someone else's network canape has no router access, so the core uses only what this machine can see: its own Wi-Fi connection, what devices announce, their makers, an internet check. Router access (Freebox today) adds extras, never a requirement.
- **Zero friction from canape.** No confirmations, no extra steps of its own. When a device demands its own approval (the LG pairing prompt), canape asks once and remembers it forever, in every home. Prefer what's automatic: devices that need no pairing (Chromecast, Roku, Sonos), the Wi-Fi password from the Keychain instead of typing it.

**The filter for every feature:** it belongs in canape if it works in one command on a network canape just joined, without a server that has to run all the time. The dashboard is fine because it's optional.

- In: more TV brands, Chromecast and Apple TV. "What's this device?" and new-device alerts. Waking a computer. Router speed and status. Named scenes ("movie mode"). Handing lights and sensors to an existing Home Assistant.
- Out: automations ("when X, do Y"), history and graphs, a plugin marketplace, anything that needs a 24/7 server, writing router settings. That's Home Assistant's job.

Supported today: LG webOS TVs, Freebox routers. Protocol notes live in `docs/` (lg.md, freebox.md, discovery.md). Read the one for the device you touch before changing its code.

## Decisions and why

- **Runs inside the network, never on a VPS.** TVs only take commands from the LAN, and Wake-on-LAN is a LAN broadcast. Remote access is Tailscale's job.
- **One home per network, keyed by the router's MAC.** `home.Load()` reads the default gateway's MAC from the ARP table (no permission needed, while recent macOS hides the Wi-Fi name from CLI tools) and returns that network's home. A new network gets an in-memory home, saved only once something is paired or `canape setup` runs, so read commands never add entries. Nothing ever overwrites another home. A pre-homes `state.json` migrates into a home named "home". When the MAC can't be read (often a VPN), `home.Load` returns `ErrUnknownNetwork`: the report runs without a home, other commands say why they can't.
- **TV brands behind one interface.** `tv.TV` is what canape does with any TV. A brand is a driver package under `internal/tv/` plus one entry in `internal/home/drivers.go`. Unsupported actions return `tv.ErrUnsupported`. Wake-on-LAN is shared (`tv.Wake`). Brand-only extras stay in the driver (`lg.TV.Call`, the LG Developer Mode app). Routers aren't abstracted: only the Freebox exists. How to add a brand: `docs/adding-a-tv.md`.
- **Computer remote, not a computer agent.** canape presses keys only on the machine it runs on, usually the one streaming to the TV. Controlling another computer would need canape running there too: a background agent fails the Direction filter. Key events need the macOS Accessibility permission (macOS prompts once). Volume uses AppleScript and needs none. Play/pause sends the space bar to the app in front: the media key goes to whatever macOS thinks is playing, often Music, which made web video unreliable in testing. `click` clicks the screen center so a web player gets focus.
- **Wake only what the user picked.** Routers remember every device ever seen and label most of them "workstation" (cameras, feeders, phones with private MACs), so canape never guesses which are computers. `canape wake add` saves one while it's online. CLI and MCP only: no dashboard page until someone needs it.
- **Fixed Wi-Fi QR, never router writes.** canape reads the SSID from the router, the user types the password once, it lives only in `state.json`. Generating Freebox guest keys would need the "settings" permission, which can change any router setting. The user chose not to grant it.
- **Stdlib first.** Dependencies only where the stdlib has nothing: `coder/websocket` (for LG, because it sends no `Origin` header and LG TVs reject one), `rsc.io/qr`, `x/net/dns/dnsmessage` (mDNS), the MCP go-sdk. Each new one needs a reason.
- **One binary, page embedded.** No frontend build. `internal/wifi/page.html` uses old CSS only: webOS 4's browser is an old Chromium (no `min()`, no flex `gap`).
- **Dashboard layout: devices first.** Each device has a page (TV, the computer canape runs on, Share Wi-Fi, plus Who's online). Home lists one card per device. On a phone, the bottom tabs are built from the devices (up to 4, then More, which is Home): switching TV and Mac during a movie stays one tap. From 900px Home is a sidebar and the page shows beside it, capped at 700px. A new device type means a new card and page. Graphite palette (neutral grays, one blue accent, tokens at the top of `internal/server/index.html`), system font, show only what's needed: 8 apps then "Show all", Network folded to one line, Wi-Fi password hidden until "Show", keyboard shortcuts in tooltips. Apps show letter tiles: LG's icon URLs return 404 even while connected.
- **Dashboard login by link.** `canape serve` prints a URL with a random key, and the same URL as a terminal QR code to scan with a phone. Opening it once moves the key into an HttpOnly, SameSite=Strict cookie. `/wifi` is public: the TV's browser has no cookie, and anyone who can reach it is already on the Wi-Fi.

## Layout

```
main.go                    calls cmd.Run, nothing else
cmd/                       the canape command: arguments and terminal output. report.go is bare `canape`, setup.go the guided flow
internal/home/             the current network's home (wake.go: computers saved for Wake-on-LAN, picked by the user): Load/Save, PairTV/ConnectTV/WakeTV via drivers.go, Status, Devices
internal/store/            state.json: homes keyed by router MAC, dashboard key, legacy migration
internal/computer/         keys on the machine canape runs on (play/pause media key, arrows, F, Esc, volume). macOS via osascript and JavaScript for Automation, so no cgo. Other systems return ErrUnsupported
internal/discovery/        network scan (SSDP, mDNS, ARP, reverse DNS, control ports), Router(), MAC(), LocalIP(), FindFreebox(). wifi.go: this machine's Wi-Fi link. internet.go: is the internet up
internal/tv/               tv.TV interface, shared types, key names, Wake-on-LAN, name matching (Match)
internal/tv/lg/            LG driver. app.go installs a web app through Developer Mode (wraps LG's ares CLI)
internal/router/freebox/   Freebox OS API client
internal/wifi/             Wi-Fi QR payload, the big-QR page, ShowOnTV
internal/qrcode/           draws any QR: SVG, terminal blocks, PNG
internal/server/           `canape serve`: login, JSON API under /api, index.html
internal/mcp/              `canape mcp`: MCP tools over stdio (go-sdk imported as sdk)
internal/version/          the version, stamped in by the release workflow (see CONTRIBUTING.md, Releasing)
```

The CLI, the dashboard and the MCP tools all go through `home`. A new capability goes into `tv.TV` (or `home`) first, then gets a CLI command, then a dashboard control and an MCP tool if it fits.

## Rules

- **Load the home where it's used:** once per command, dashboard request or MCP call. A copy kept around goes stale and its next save wipes what other commands wrote.
- **Under `canape mcp`, stdout is the protocol.** Shared code must not print to stdout: return strings, or write to stderr.
- `state.json` holds pairing keys and Wi-Fi passwords: mode 0600, never log its contents. Its path: `~/.canape/state.json`, always. A file in the old location (the OS config dir) moves there on first load.
- No real IPs, MACs, SSIDs or keys in code, tests or docs.
- Ask the user to run `canape scan` instead of asking them for device details.
- Commits: one-line Conventional Commits.

## Roadmap

[ROADMAP.md](ROADMAP.md) is the plan. Check items off there when they land, and add what's left to verify on real hardware.

## Out of scope

What fails the filter in Direction, plus: cloud servers or relays, tracking what people browse, rooting TVs.
