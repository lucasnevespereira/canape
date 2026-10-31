# Roadmap

Where canape is going, in order. Every item passes the filter in [AGENTS.md](AGENTS.md#direction): it works in one command on a network canape just joined, with nothing that has to keep running.

## Who it's for

Someone who has never heard of an IP address should be able to use canape. That shapes every feature:

- **Plain words.** "Your internet is fine, the Wi-Fi in this room is weak", not "packet loss 12%". Technical details are one tap away, never in the way.
- **Every problem comes with what to do next.** A message that only says what's wrong is half a message.
- **No terminal for everyday use.** The terminal is for setup and for people who like it. Everything else works from the phone.
- **Safe by default.** canape reads from the router and never changes its settings. When something needs changing, it says where and how, and opens the right page.
- **In the user's language.** French first (Freebox homes), then more.

## Done (v0.1 to v0.3)

- [x] LG webOS TVs: power, volume, inputs, apps, remote keys, typing, web pages, notifications
- [x] Freebox: pairing and the device list
- [x] Wi-Fi QR code in the terminal, on the TV and in the web dashboard
- [x] Web dashboard with a login QR, organized by device: phone tabs built from the devices, a sidebar on a computer, add to home screen
- [x] Computer remote on macOS: play, pause, skip, volume, full screen on the machine canape runs on
- [x] `canape wake`: wake a computer the user saved
- [x] Claude through MCP (`canape mcp`)
- [x] Homes: one setup per network, picked automatically
- [x] `canape setup`, help grouped by task
- [x] The network report: bare `canape` shows the Wi-Fi (band, signal, security), the box, whether the internet works, and every device grouped by kind, then what to do next. Works without router access
- [x] A moved TV is found again, setup names TVs it can't control yet
- [x] `canape version`, releases with binaries, Homebrew updated automatically

## Still to check on real hardware

- `canape setup` on a second network (only simulated so far)
- The MCP tools from Claude itself (only driven by raw test messages so far)
- The dashboard added to an iPhone home screen: it should log itself in on first launch
- Play/pause with the space bar and Click the video on a web player
- Typing on the computer from the dashboard, with accents, on a French keyboard
- Pressing Enter to skip the Freebox pairing in `canape setup`
- `install.sh` on a Raspberry Pi (only run on a Mac so far)
- The network report with a VPN on (it should still run, without a home)
- The network report on a network without router access (a friend's box, a hotel), and on Linux where the Wi-Fi line is empty for now

## Next

- **`canape check`: is it the internet or the Wi-Fi?** The question everyone asks. The report already answers it in one line. A command of its own skips the device scan, so the answer comes in a second. Also in the dashboard as a big "Check my connection" button.
- **No typing the Wi-Fi password.** Read it from the Mac's Keychain for the network you're on, after one macOS prompt.
- **Setup from the phone.** Pair the TV and the router, save the Wi-Fi, all from the dashboard. Today it needs `canape setup` in a terminal, which loses most people.
- **French.** The dashboard and the CLI in French, picked from the phone's or the computer's language.
- **Devices that need no pairing first:** Chromecast, Roku, Sonos. Zero friction at anyone's house.
- **More TVs.** Samsung (Tizen) and Sony Bravia, each built in front of a real TV. `canape scan` already recognizes them.
- **Android TV**, through ADB or the Android TV remote protocol. Covers projectors like XGIMI and the Freebox Player.
- **Run in the background.** `canape serve` as a service (launchd on macOS, systemd on Linux and Pi), so the dashboard is always there.

## A network helper for everyone

### When something's wrong

- **Check my connection.** See Next. The answer names the culprit (box, internet provider, Wi-Fi, this device) and the fix: "Unplug the box for 10 seconds, then wait 3 minutes", "Your provider has an outage, nothing to do on your side".
- **Weak Wi-Fi.** Signal strength per device from the router, in words ("good", "weak, move closer to the box"), and whether a device is on 2.4 GHz (reaches far, slower) or 5 GHz (fast, shorter range).
- **Is my internet fast enough?** A speed test with a meaning: "Enough for 4K on two TVs at once" instead of "312 Mbps".
- **My phone can't find the TV.** Checks the two usual causes: the phone is on another network (guest Wi-Fi, or 4G), or the TV is asleep. Then says which one.
- **Restart in the right order.** A short guide when nothing else works: the box first, then the device, with the waiting times.

### Knowing your network

- **Who's on my Wi-Fi?** Every device with a readable name and a kind (phone, TV, speaker, camera, smart plug), guessed from its maker and what it announces. "lwip0" becomes "Tuya smart plug".
- **Name devices once.** "PETLIBRO-Air Smart Feeder" becomes "Cat feeder", saved in the home, shown everywhere.
- **Spot an unknown device.** Devices nobody named stand out, with a hint about what they probably are, so a stranger on the Wi-Fi is easy to notice.
- **New device alert** while `canape serve` runs: "A new device joined: iPhone".
- **Find a device.** `canape find printer` gives its address and opens its own web page when it has one (printers, cameras, home servers).
- **Which box do I have?** The router's model and a button to its settings page, without knowing its address.
- **A map of the house,** devices grouped by kind, readable at a glance.

### Keeping it safe

A read-only checkup, each finding with where to fix it:

- **Wi-Fi security:** the encryption in use (WPA3 good, WPA2 fine, WEP or open network to fix now) and whether the saved password is too short.
- **Open to the internet:** ports the router forwards from the internet, and remote access to the box, explained ("your printer can be reached from the internet").
- **Updates:** whether the box reports a pending firmware update.
- **Guests apart:** whether a guest network keeps visitors away from your own devices.

### Sharing

- **A printable Wi-Fi card** with the QR code and the password, to stick on the fridge or put in a guest room.
- **The Wi-Fi QR on the TV** for a room full of guests. Done.

### Understanding

- **Short explanations on tap** for the words that show up: what an IP address, a MAC address, DNS, 2.4 and 5 GHz are, one sentence each.

## Later

- A Mac menu bar app that starts canape and shows the login QR, for people who never open a terminal
- Computer keys on Linux (playerctl, xdotool) and Windows (media keys)
- Wake a computer from the dashboard, when someone needs it (the command exists)
- Named scenes, like "movie mode": TV on, Apple TV input, volume 15
- Hand lights and sensors to Home Assistant when a home already runs it
- Chromecast and Apple TV
- Docker image, for NAS and home servers

## Not planned

Automations, history and graphs, a plugin marketplace, anything that needs a server running all the time, and writing router settings. Pausing a child's internet, for example, would mean changing router settings: canape will point to the router's own parental controls instead. That's Home Assistant's and the router's job. See [AGENTS.md](AGENTS.md#direction).
