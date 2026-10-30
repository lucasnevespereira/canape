# canape

A self-hosted home control center. One Go binary runs on an always-on machine at home (a Raspberry Pi, or a Mac while developing). It serves a phone-first web dashboard and talks to the home's devices over the local network.

It starts with two devices, an LG webOS TV and a Freebox router (Pop). Planned for open source.

## Decisions and why

- **Runs inside the home network, never on a VPS.** The LG API and Wake-on-LAN only work from the home network. Exposing the Freebox API to the internet would put router admin on the public internet. Remote access is Tailscale's job, not ours.
- **One binary, web page embedded** with `embed`. No separate frontend build, no nginx. A plain HTML/JS page calls the binary's JSON API.
- **Stdlib first.** `net/http` with Go 1.22+ method routing, `embed`, `encoding/json`, `crypto/hmac`. Add a dependency only where the stdlib has nothing: a WebSocket client (for the LG) and maybe QR generation. Each new dependency needs a reason.
- **One home per network, keyed by the router's MAC.** canape works wherever you are: your home, your sister's. `home.Load()` reads the default gateway's MAC from the ARP table (no permission needed; recent macOS hides the Wi-Fi name from CLI tools) and returns that network's home. A new network gets an in-memory home that's only saved once something is paired, so read commands never add entries. Nothing ever overwrites another home. A pre-homes `state.json` migrates into a home named "home" on first run.
- **Only the devices we have.** No plugin system or device interfaces until a second TV brand or router actually shows up.
- **Login from day one.** Anyone on the Wi-Fi, guests included, can reach the dashboard. `canape serve` prints a link with a random key; opening it once moves the key into an HttpOnly, SameSite=Strict cookie. No password to type on a phone. Delete `dashboard.key` from the state file to log every device out.
- **Docker is packaging, not the runtime.** It ships later as a thin wrapper around the binary and needs `network_mode: host` (for WoL broadcasts and discovery). On a Mac, develop with `go run .`, because Docker Desktop host networking is unreliable.
- **Fixed Wi-Fi QR, not generated guest keys.** The user chose it: creating guest keys needs the Freebox "settings" permission, which can change any router setting. canape reads the SSID from the box (allowed without that permission; the API masks the password as `********`), the user types the password once (`canape wifi set`), and it lives only in `state.json`. canape never writes Wi-Fi settings. Generated keys stay possible later.
- **MCP comes later**, as a second interface in the same binary that calls the same functions as the HTTP handlers.

## Layout

```
main.go                    calls cli.Run, nothing else
internal/cli/              commands and terminal output (tv, wifi, freebox, scan)
internal/store/            state.json: load, save, path
internal/home/             saved state + live devices: ConnectTV, WakeTV, Status, Devices. CLI, server and MCP all go through it
internal/discovery/        network scan: SSDP, mDNS, ARP, reverse DNS, control ports. LocalIP
internal/tv/               what every TV brand shares (name matching today)
internal/tv/lg/            LG webOS client, plus the Developer Mode app installer (app.go, wraps LG's ares CLI)
internal/tv/lg/register.json  pairing payload (from PyWebOSTV). The signature covers the "signed" block, so edit neither.
internal/router/freebox/   Freebox OS API client
internal/wifi/             Wi-Fi QR: payload, SVG, terminal, PNG, the big-QR page (page.html), ShowOnTV
internal/server/           `canape serve`: login link, JSON API under /api/tv, public /wifi page, index.html
internal/mcp/              `canape mcp`: MCP tools over stdio (official go-sdk, imported as sdk)
docs/                      longer notes (discovery.md)
```

`page.html` is kept to old CSS on purpose: webOS 4's browser is an old Chromium (no min(), no flex gap).

State lives in `state.json` inside `CANAPE_DATA`, or by default the OS user config dir (`~/Library/Application Support/canape` on macOS, `~/.config/canape` on Linux), so the CLI works from any directory. Docker will set `CANAPE_DATA=/data`. Shape: `{"homes": {"<router mac>": {name, tv: {brand, ip, mac, key}, wifi, freebox}}, "dashboard": {key}}`. It holds pairing keys and Wi-Fi passwords: write it with mode 0600 and never log tokens.

**Under `canape mcp`, stdout is the protocol.** Code shared with the MCP tools must not print to stdout: return strings, or write to stderr.

The CLI is the test bench: each feature lands as a `canape tv ...` command first, and the web page calls the same functions later.

## LG webOS protocol (SSAP)

- **Discovery:** SSDP M-SEARCH with `ST: urn:lge-com:service:webos-second-screen:1`. The responder's address is the TV. Commands to find and probe devices by hand are in `docs/discovery.md`.
- WebSocket at `ws://<tv>:3000`. Newer firmware may require `wss://<tv>:3001` with a self-signed cert. Try wss first. The dev TV (55SM8200, webOS 4.1) accepts both.
- **No `Origin` header.** The TV closes any connection that sends one (close code 1008 "invalid origin"). That's why `golang.org/x/net/websocket`, which always sends it, can't be used; `github.com/coder/websocket` doesn't send it.
- **Pairing:** send a `register` message with a permissions manifest. The TV shows an accept prompt and replies with a `client-key`. Store it and send it in later `register` messages so the TV skips the prompt.
- **Requests:** `{"type":"request","id":"<n>","uri":"ssap://..."}` with an optional `payload`. Replies carry the same `id`.
- Useful URIs: `ssap://system/turnOff`, `ssap://audio/setVolume`, `ssap://audio/volumeUp|volumeDown`, `ssap://audio/setMute`, `ssap://tv/switchInput`, `ssap://system.launcher/launch` (by app id), `ssap://system.launcher/open` (opens a URL in the browser), `ssap://system.notifications/createToast`, `ssap://com.webos.applicationManager/getForegroundAppInfo`.
- Remote buttons (home, back, arrows) go through a second socket from `ssap://com.webos.service.networkinput/getPointerInputSocket`.
- **Power on is not SSAP.** When the TV is off, the socket is dead. Send a Wake-on-LAN magic packet to the TV's MAC as a UDP broadcast. This only works if the TV has "Turn on via Wi-Fi" enabled (the settings path varies by webOS version).

## Freebox OS API

Official docs: https://dev.freebox.fr/sdk/os/

- **Discovery:** `GET http://mafreebox.freebox.fr/api_version` returns `api_base_url` and `api_version`. Build URLs from it instead of hardcoding a version. The dev box is a Pop (`Freebox v8`, API 16.1).
- **App authorization (once):** `POST <base>/login/authorize/` with `app_id`, `app_name`, `app_version` and `device_name` returns an `app_token` and a `track_id`. The user approves on the box's small screen (arrows to "Oui"), then poll `GET <base>/login/authorize/<track_id>` until `granted`. Known Pop bug (dev.freebox.fr FS#37677): the box display can stay on "Authentification en cours" after approval, but polling still sees `granted`.
- **Plain HTTP on the LAN.** HTTPS uses Free's own CA, which isn't in system trust stores. HTTP works locally.
- **Session (each run):** `GET <base>/login/` returns a `challenge`. Then `password = hex(hmac_sha1(key=app_token, msg=challenge))`. `POST <base>/login/session/` returns a `session_token`, sent as the `X-Fbx-App-Auth` header. Sessions expire: on an auth error, open a new session and retry once.
- **Permissions:** the user grants them per app in the Freebox OS web UI. Changing Wi-Fi settings (guest keys) needs the settings permission. Surface missing permissions clearly instead of failing silently.
- Endpoints we use: `lan/browser/pub/` (connected devices), `wifi/custom_key/` (temporary guest Wi-Fi keys), `call/log/` (landline call log).
- **Guest keys** (not in the official docs; field names from hacf-fr/freebox-api and Procsiab/uaifai, unverified on the Pop): `POST wifi/custom_key/` with `description`, `key`, `max_use_count`, `duration` (seconds) and `access_type` (`net_only` keeps guests off the LAN, `full` doesn't). A guest key is an extra password on the main SSID; read the SSID from `GET wifi/bss/` (`[0].bss_params.ssid` on recent APIs, `config.ssid` in the old docs).

## Guest Wi-Fi QR

Format: `WIFI:T:WPA;S:<ssid>;P:<key>;;`. Escape `\ ; , : "` in the SSID and key with a backslash. The QR always encodes a temporary Freebox guest key, never the main Wi-Fi password.

## Plan

Build one small step at a time. Each step should be easy to follow and explain on its own. Check items off as they land.

- [x] LG: pair, store the client-key, toast, off, on (Wake-on-LAN), volume, raw `call`. Tested on the dev TV.
- [x] LG: input switch, launch app, open URL. Tested on the dev TV.
- [x] HTTP server + `web/index.html` with TV controls, plus a login link. Works on a phone. Home/Back keys work (`Button`, second socket).
- [x] Freebox: authorize, session, list connected devices. Paired with the dev Pop; `settings` permission deliberately not granted.
- [x] Wi-Fi QR: `canape wifi set`, terminal QR, public `/wifi` page, dashboard card with "Show on TV" (`system.launcher/open`). Scanned with a phone and shown on the dev TV.
- [ ] Packaging: install script (systemd and launchd), Dockerfile, compose with host networking
- [x] Repo layout: `internal/` packages, `canape scan` replaces cmd/scan, MIT license, CI.
- [x] Homes: one per network (`canape homes`, rename, forget), automatic migration. Migrated the dev config; new-network flow checked with a simulated state.
- [ ] TV interface with LG as the first driver; Samsung/Sony detection in scan.
- [ ] CLI: `canape setup`, bare `canape` status, renames (`notify`, `key`, `canape devices`).
- [ ] Docs: README quick start, docs/, CONTRIBUTING (adding a TV brand).
- [x] MCP interface over the same functions. `canape mcp` with 12 tools (TV control, Wi-Fi on TV, network devices). Verified over stdio (tools/list, tv_status, network_devices); not yet used from Claude.
- [x] Wi-Fi TV app: `canape tv install-app` wraps LG's ares CLI (`@webos-tools/cli`): registers the TV as `canape-tv` from the paired IP (port 9922, user prisoner), fetches the Dev Mode key once (ares stores it in `~/.ssh/canape-tv_webos`), packages, installs, launches. Installed and working on the dev TV. Installing over SSH without ares would drop the Node requirement.
- [x] `canape wifi tv`: opens the Wi-Fi page in the TV browser without `canape serve`. A throwaway server on a random port answers the TV's single request, then exits; the inline-SVG QR stays on screen. Preferred over the TV app, which is an optional offline extra (Dev Mode expires).
- [x] Typing on the TV: `canape tv type` and a dashboard text field (`ime/insertText`), plus an OK key. Used to log into Developer Mode on the dev TV. Dev Mode sessions expire; auto-extending them is a possible follow-up.
- Later, maybe: AdGuard Home pairing for DNS logs, Apple TV (pyatv protocol), XGIMI Android TV (ADB)

## Out of scope

Running on a cloud server, cloud relays, TV brands and routers nobody has asked for, rooting the TV.
