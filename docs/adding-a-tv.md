# Adding a TV brand

canape talks to every TV through one interface, `tv.TV` in `internal/tv/tv.go`. A brand is a driver package that satisfies it, plus one line that tells canape how to find and connect to that brand. You need the TV in front of you: every brand has surprises the docs don't mention.

## 1. Look at the TV first

On the TV's network, run `canape scan` and keep the output. It shows the TV's IP, MAC, model, what it announces and which control ports answer. Add a rule to `Device.Kind()` in `internal/discovery/discovery.go` so scan names the brand, with a case in `TestKindTVBrands`.

## 2. Write the driver

Create `internal/tv/<brand>/` with:

- a type whose methods satisfy `tv.TV`. Add `var _ tv.TV = (*TV)(nil)` so the compiler checks it.
- `Discover(timeout time.Duration) (ip string, err error)`: find the TV on the network.
- `Connect(ctx context.Context, ip, key string, prompted func()) (*TV, string, error)`: dial and authenticate. With an empty key, the TV should ask the user to accept canape; call `prompted` while it waits, and return the key to save.

Things a TV can't do return `tv.ErrUnsupported`. Remote keys arrive as `tv.Keys` (`HOME`, `BACK`, `UP`, `DOWN`, `LEFT`, `RIGHT`, `ENTER`); map them to the brand's codes. `Current()` returns the ID of the input or app on screen, matching the IDs `Inputs()` and `Apps()` return. Turning on is shared: `tv.Wake` sends Wake-on-LAN to the MAC saved at pairing.

Keep brand-only extras in your package, like `lg.TV.Call`.

## 3. Register it

Add an entry to `drivers` in `internal/home/drivers.go`: a name shown to users, your `Discover`, and a `connect` wrapper. Pairing, the CLI, the dashboard and the MCP tools pick it up from there.

## 4. Try it

```bash
go run . setup        # discovers and pairs
go run . tv status
go run . tv input
go run . tv app youtube
go run . tv key home
go run . wifi tv
```

Then write `docs/<brand>.md` (TV settings to change, protocol notes) and add the brand to the table in the README.

## Where to start, by brand

| Brand | Protocol | Pairing |
|---|---|---|
| Samsung (Tizen, 2016+) | WebSocket on `wss://<tv>:8002/api/v2/channels/samsung.remote.control`, keys as `KEY_HOME`… | The TV prompts, then hands out a token |
| Sony Bravia | "IP control" REST API on `http://<tv>/sony/...` | A pre-shared key (PIN) set in the TV's network settings |
| Android / Google TV | ADB over Wi-Fi (port 5555), or the Android TV Remote protocol | Network debugging, or an on-screen code |
| Apple TV | Companion protocol, see [pyatv](https://pyatv.dev) | An on-screen PIN |
