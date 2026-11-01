# Apple TV

canape talks to the Apple TV directly over the Wi-Fi with the Companion protocol, the one the iPhone's remote uses. Apple doesn't document it. This follows what [pyatv](https://pyatv.dev) worked out.

## Why not through the TV

An Apple TV plugged into an LG TV can't be reached through the TV. With SimpLink (HDMI-CEC) on, the LG's physical remote controls the Apple TV, but keys sent to the LG over the network (play, pause, OK, arrows) never leave the TV. Tested on a 2019 LG (webOS 4) with an Apple TV 4K.

## Pairing

```bash
canape appletv pair
```

The Apple TV shows a 4-digit code, you type it, and canape saves keys in the home. `canape setup` offers the same step. To unpair, remove canape in the Apple TV's Settings > Remotes and Devices > Remote App and Devices.

What happens, for whoever changes this code:

1. **Find it.** mDNS service `_companion-link._tcp`. Its SRV record gives the port (it changes, 49152 and up). `canape scan` keeps it in `Device.CompanionPort`.
2. **Pair setup** (once). HomeKit's pair setup in Companion frames: SRP-6a with the 3072-bit group, SHA-512, user `Pair-Setup` and the code as password. Then canape and the Apple TV swap Ed25519 public keys, encrypted with ChaCha20-Poly1305 (nonces `PS-Msg05`, `PS-Msg06`).
3. **Pair verify** (every connection). X25519 key exchange, each side signs it with its Ed25519 key. After that every frame is encrypted with keys from HKDF-SHA512 (`ClientEncrypt-main`, `ServerEncrypt-main`), the nonce being a per-direction counter.
4. **Open a session.** `_systemInfo`, `_touchStart`, `_sessionStart`. A full connect takes about 80 ms on a home Wi-Fi, so the dashboard connects for each press.

## Frames and messages

A frame is a type byte, a 3-byte big-endian length, then the payload: 3 and 4 for pair setup, 5 and 6 for pair verify, 8 for encrypted messages. Messages are OPACK, Apple's binary JSON, with `_i` (the command), `_t` (1 event, 2 request, 3 response), `_x` (a transaction number) and `_c` (the content).

| What | Message |
|---|---|
| Remote button | `_hidC` with `_hBtS` 1 (down) then 2 (up), and `_hidC`: up 1, down 2, left 3, right 4, menu 5, select 6, home 7, volume up 8, volume down 9, sleep 12, wake 13, play/pause 14 |
| Skip | `_mcc` with `_mcc` 7 and `_skpS` in seconds, negative to go back |

## Not working yet

- **App list and launching.** `FetchLaunchableApplicationsEvent` gets no answer on the tested Apple TV, even after `_touchStart`. pyatv sends extra details in pair setup (a name, a model, an account ID), which may be what it needs.
- **What's playing.** Needs the MRP protocol tunneled over AirPlay, a much bigger piece.
