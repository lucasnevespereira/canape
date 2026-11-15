# Android TV

canape controls Android TVs and Android projectors (XGIMI and the like) over ADB, the same channel Android developers use. canape speaks it itself, so there's no `adb` to install.

## Turn on network debugging, once

1. Settings, Device Preferences, About. Select **Build** seven times, until it says you're a developer.
2. Back in Device Preferences, open **Developer options** and turn on **USB debugging**. Some TVs also have **Network debugging** or **ADB over network**: turn it on too.
3. On the computer: `canape setup`. The TV asks "Allow debugging?". Tick **Always allow from this computer**, then OK.

`canape scan` shows the TV with port 5555 open once it's on. On TVs where the port is closed after a reboot, it needs a one-time `adb tcpip 5555` over USB, which canape can't do for you.

## What works

Power off (standby), volume and mute, the remote's arrows, OK, Back and Home, typing, opening apps and web pages, which app is on screen. Tested on an XGIMI Halo+ (Android TV 11).

Not yet: HDMI inputs, and on-screen messages (Android TV launchers don't show them). `canape tv on` sends Wake-on-LAN, which many projectors ignore in deep standby.

## How it talks to the TV

ADB on TCP 5555. canape makes a 2048-bit RSA key at pairing and keeps it in `state.json` as the TV's key. The TV checks a signature with it each time, and asks the user once to trust it.

Each action is one shell command: `input keyevent` for keys, `monkey` to open an app, `am start` for a URL, `dumpsys audio` to read the volume and `cmd media_session volume` (`media volume` on Android 10) to set it. The volume tools print nothing on some Android 11 firmware, so canape never reads from them. The app list is every app with a TV launcher entry. The shell can't read app names, so a short list of known apps get a title and the others show their package name.

Network debugging gives full control of the TV to anyone on the Wi-Fi who it trusts. Only computers you accepted on screen get in.
