# LG webOS TVs

## Prepare the TV

1. **Turn on "Turn on via Wi-Fi".** Without it, canape can turn the TV off but never back on. The menu moves between webOS versions: look under Settings → All Settings → Connection → Mobile Connection Management → TV On With Mobile, or General → Mobile TV On.
2. **Quick Start+** (General) keeps the network awake in standby, which makes turning the TV on more reliable. It uses a little more standby power.
3. **A fixed IP** helps: give the TV a DHCP reservation on your router. If the IP changes anyway, run `canape tv pair` again.

Then run `canape setup` and accept the prompt on the TV.

## Fewer ads on the home screen

No canape needed. On the TV:

1. Settings → General → System (or Home Settings): turn off **Home Promotion**, **Content Recommendation** and **Home Auto Launch**.
2. Settings → General → System → **User Agreements**: decline viewing information, voice information and interest-based ads.

Menu names change between webOS versions. This removes the big banner and most suggested content.

To go further, run [AdGuard Home](https://github.com/AdguardTeam/AdGuardHome) as your network's DNS: it blocks ad and tracking domains for every device and shows what the TV contacts. Add blocks gradually, since some LG domains also serve the app store and updates. Network blocking can't remove YouTube ads: they come from the same servers as the videos.

## A Wi-Fi app on the TV

You probably don't need this: `canape wifi tv` shows the QR on the TV in one command. The app is for showing it with no computer at all: a "Wi-Fi" tile in the app bar with your QR baked in, installed through LG's Developer Mode.

One-time setup on the TV (LG requires it):

1. Create a free account at [webostv.developer.lge.com](https://webostv.developer.lge.com) with an email and password. Accounts made with "Continue with Google" can't sign in on the TV.
2. On the TV, open **Developer Mode** (`canape tv app developer`, or the LG Content Store), sign in, turn on **Dev Mode Status**, and let the TV restart. `canape tv type` types the email and password for you: select the field with the remote, then run it.
3. Open Developer Mode again and turn on **Key Server**. It shows a passphrase.

Then on your computer:

```bash
npm install -g @webos-tools/cli     # LG's CLI, once
canape tv install-app
```

The first run asks for the passphrase. Run it again after changing the Wi-Fi password. Developer Mode lasts a limited time: press **Extend** in the Developer Mode app before it runs out, or the TV removes the app.

## How canape talks to the TV

LG calls the protocol SSAP: JSON over a WebSocket.

- **Discovery:** an SSDP search for `urn:lge-com:service:webos-second-screen:1`. The TV answers with its address. See [discovery.md](discovery.md) to do it by hand.
- **Connection:** `wss://<tv>:3001` (self-signed certificate), falling back to `ws://<tv>:3000`. Recent firmware may close the plain port.
- **No `Origin` header.** The TV closes any connection that sends one (close code 1008, "invalid origin"). That's why canape uses `github.com/coder/websocket` and not `golang.org/x/net/websocket`, which always sends it.
- **Pairing:** a `register` message carrying a signed permissions manifest (`internal/tv/lg/register.json`, from PyWebOSTV). The TV shows a prompt and answers with a `client-key`, which later `register` messages send to skip the prompt.
- **Requests:** `{"type":"request","id":"<n>","uri":"ssap://...","payload":{...}}`. The reply carries the same `id`. `canape tv call <uri> [json]` sends any of them, e.g. `canape tv call audio/getVolume`.
- **Remote keys** go through a second socket, from `ssap://com.webos.service.networkinput/getPointerInputSocket`, as plain-text frames: `type:button\nname:HOME\n\n`.
- **Typing:** `ssap://com.webos.service.ime/insertText`, into whatever field has focus.
- **Inputs are apps.** The foreground app is `com.webos.app.hdmi1` when HDMI 1 is on screen; canape maps it back to the input ID `HDMI_1`.
- **Turning on isn't SSAP:** a TV that's off has no socket. canape sends a Wake-on-LAN packet to its MAC.

Useful URIs: `system/turnOff`, `audio/getVolume`, `audio/setVolume`, `audio/setMute`, `tv/getExternalInputList`, `tv/switchInput`, `com.webos.applicationManager/listApps`, `com.webos.applicationManager/getForegroundAppInfo`, `system.launcher/launch`, `system.launcher/open`, `system.notifications/createToast`.
