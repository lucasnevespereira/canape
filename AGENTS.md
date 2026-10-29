# canape

A self-hosted home control center. One Go binary runs on an always-on machine at home (a Raspberry Pi, or a Mac while developing). It serves a phone-first web dashboard and talks to the home's devices over the local network.

It starts with two devices, an LG webOS TV and a Freebox router (Pop). Planned for open source.

## Decisions and why

- **Runs inside the home network, never on a VPS.** The LG API and Wake-on-LAN only work from the home network. Exposing the Freebox API to the internet would put router admin on the public internet. Remote access is Tailscale's job, not ours.
- **One binary, web page embedded** with `embed`. No separate frontend build, no nginx. A plain HTML/JS page calls the binary's JSON API.
- **Stdlib first.** `net/http` with Go 1.22+ method routing, `embed`, `encoding/json`, `crypto/hmac`. Add a dependency only where the stdlib has nothing: a WebSocket client (for the LG) and maybe QR generation. Each new dependency needs a reason.
- **Only the devices we have.** No plugin system or device interfaces until a second TV brand or router actually shows up.
- **Login from day one.** Anyone on the Wi-Fi, guests included, can reach the dashboard. `canape serve` prints a link with a random key; opening it once moves the key into an HttpOnly, SameSite=Strict cookie. No password to type on a phone. Delete `dashboard.key` from the state file to log every device out.
- **Docker is packaging, not the runtime.** It ships later as a thin wrapper around the binary and needs `network_mode: host` (for WoL broadcasts and discovery). On a Mac, develop with `go run .`, because Docker Desktop host networking is unreliable.
- **MCP comes later**, as a second interface in the same binary that calls the same functions as the HTTP handlers.

## Layout

```
main.go          entry point, usage, state file
tv.go            `canape tv ...` CLI commands
server.go        `canape serve`: login, JSON API under /api/tv, embeds web/index.html
web/index.html   dashboard, one file, no build step
lgtv/            LG webOS client. Named methods (TurnOff, SetVolume, Launch...) are what the CLI and server call.
lgtv/register.json  pairing payload (from PyWebOSTV). The signature covers the "signed" block, so edit neither.
cmd/scan/        network scanner: `go run ./cmd/scan` lists devices, IPs, MACs, models, open ports. Ask the user to run it instead of asking them for device details.
freebox/         Freebox OS API client (not yet)
```

State lives in `state.json` inside `CANAPE_DATA`, or by default the OS user config dir (`~/Library/Application Support/canape` on macOS, `~/.config/canape` on Linux), so the CLI works from any directory. Docker will set `CANAPE_DATA=/data`. The file holds the TV client-key, the dashboard login key, and later the Freebox app_token. Write it with mode 0600 and never log tokens.

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
- **App authorization (once):** `POST <base>/login/authorize/` with `app_id`, `app_name`, `app_version` and `device_name` returns an `app_token` and a `track_id`. The user approves on the box, then poll `GET <base>/login/authorize/<track_id>` until `granted`. The Pop has no front screen, so check the docs for how a Pop owner approves.
- **Session (each run):** `GET <base>/login/` returns a `challenge`. Then `password = hex(hmac_sha1(key=app_token, msg=challenge))`. `POST <base>/login/session/` returns a `session_token`, sent as the `X-Fbx-App-Auth` header. Sessions expire: on an auth error, open a new session and retry once.
- **Permissions:** the user grants them per app in the Freebox OS web UI. Changing Wi-Fi settings (guest keys) needs the settings permission. Surface missing permissions clearly instead of failing silently.
- Endpoints we use: `lan/browser/pub/` (connected devices), `wifi/custom_key/` (temporary guest Wi-Fi keys), `call/log/` (landline call log).

## Guest Wi-Fi QR

Format: `WIFI:T:WPA;S:<ssid>;P:<key>;;`. Escape `\ ; , : "` in the SSID and key with a backslash. The QR always encodes a temporary Freebox guest key, never the main Wi-Fi password.

## Plan

Build one small step at a time. Each step should be easy to follow and explain on its own. Check items off as they land.

- [x] LG: pair, store the client-key, toast, off, on (Wake-on-LAN), volume, raw `call`. Tested on the dev TV.
- [x] LG: input switch, launch app, open URL. Tested on the dev TV.
- [x] HTTP server + `web/index.html` with TV controls, plus a login link. Works on a phone. Home/Back keys work (`Button`, second socket).
- [ ] Freebox: authorize, session, list connected devices
- [ ] Freebox: create a guest key, show its QR on the phone or send it to the TV (`system.launcher/open`)
- [ ] Packaging: install script (systemd and launchd), Dockerfile, compose with host networking
- [ ] MCP interface over the same functions
- Later, maybe: webOS app tile showing the guest QR (needs LG Developer Mode), AdGuard Home pairing for DNS logs, Apple TV (pyatv protocol), XGIMI Android TV (ADB)

## Out of scope

Running on a cloud server, cloud relays, TV brands and routers nobody has asked for, rooting the TV.
