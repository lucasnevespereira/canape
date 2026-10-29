# Finding your devices

How to find the TV and the Freebox on your network, and check they answer, before writing any code. Run these from a machine on the same Wi-Fi. Everything here is read-only.

## Quick way

```bash
go run ./cmd/scan
```

This prints every device on the network with its IP, MAC, model and open control ports. For the LG TV, the IP and MAC are what canape needs. It takes about 6 seconds and does everything below automatically:

- SSDP search: finds the LG TV, the Freebox and other UPnP devices
- mDNS search: finds the Apple TV, Macs, Google Cast, Spotify Connect
- Sends one UDP packet to every address, so silent devices show up in the ARP table with their MAC
- Reverse DNS: the Freebox resolves device hostnames (`lgwebostv.home`, `freebox-player-pop.home`)
- Port probe: 3000/3001 (LG control), 5555 (ADB), 8009 (Google Cast)

The rest of this page shows how to do each step by hand.

## 1. Your network

```bash
ipconfig getifaddr en0                 # your Mac's IP, e.g. 192.168.1.10
route -n get default | grep gateway    # the router, 192.168.1.254 on a Freebox
```

On Linux: `ip addr` and `ip route`.

## 2. The Freebox

```bash
curl -s http://mafreebox.freebox.fr/api_version
```

No auth needed. It returns something like:

```json
{"box_model_name":"Freebox v8 (r1)","api_base_url":"/api/","api_version":"16.1",
 "https_available":true,"https_port":47860,"device_type":"FreeboxServer8,1", ...}
```

- `Freebox v8` is the **Pop**.
- `api_version` "16.1" means calls go to `/api/v16/...`. Read the version from here rather than hardcoding it.
- `api_domain` and `https_port` are for remote HTTPS access. canape talks to the box over the LAN, so it doesn't use them.

## 3. The LG TV

### Discover it with SSDP

LG TVs answer an SSDP search (UPnP discovery over UDP multicast) for their "second screen" service. Send the search and print who replies:

```bash
python3 -I - <<'EOF'
import socket
msg = ('M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: "ssdp:discover"\r\nMX: 2\r\n'
       'ST: urn:lge-com:service:webos-second-screen:1\r\n\r\n').encode()
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(3)
s.sendto(msg, ('239.255.255.250', 1900))
try:
    while True:
        data, addr = s.recvfrom(4096)
        print(addr[0]); print(data.decode(errors='replace'))
except socket.timeout:
    pass
EOF
```

A reply looks like this:

```
192.168.1.20
HTTP/1.1 200 OK
Location: http://192.168.1.20:1088/
Server: WebOS/4.1.0 UPnP/1.0
ST: urn:lge-com:service:webos-second-screen:1
```

- The sender address is the TV's IP.
- `Server: WebOS/4.1.0` gives the webOS version. 4.x is a 2019 model.
- The TV only answers while it's on, or in standby with "Turn on via Wi-Fi" enabled.

canape can use the same search to find the TV automatically.

### Model name

The `Location` URL serves a UPnP description:

```bash
curl -s http://<tv-ip>:1088/ | grep -oE '<(friendlyName|modelNumber|manufacturer)>[^<]*'
```

```
<friendlyName>[LG] webOS TV SM8200PLA
<manufacturer>LG Electronics
<modelNumber>55SM8200PLA
```

### MAC address (needed for Wake-on-LAN)

Once you've talked to the TV (the SSDP step does that), it's in your ARP cache:

```bash
arp -n <tv-ip>
# ? (192.168.1.20) at aa:bb:cc:d:ee:ff on en0
```

macOS drops leading zeros: `aa:bb:cc:d:ee:ff` is `aa:bb:cc:0d:ee:ff`. The Freebox device list shows the MAC too.

### Which control port works

The TV's control API (SSAP) is a WebSocket on port 3000 (plain) or 3001 (TLS, self-signed cert). Check both ports are open:

```bash
nc -z -G 2 <tv-ip> 3000 && echo open
nc -z -G 2 <tv-ip> 3001 && echo open
```

Then check each one accepts a WebSocket handshake:

```bash
for u in http://<tv-ip>:3000/ https://<tv-ip>:3001/; do
  curl -sk -m 3 -o /dev/null -w "$u %{http_code}\n" \
    -H 'Connection: Upgrade' -H 'Upgrade: websocket' \
    -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' $u
done
```

`101` means the WebSocket handshake succeeded. curl then reports a timeout and exits with code 28, because the socket stays open. That's expected.

On recent firmware, plain port 3000 may refuse connections. That's why canape tries `wss://:3001` first.

## 4. Pin the IPs

In Freebox OS (http://mafreebox.freebox.fr) → Paramètres de la Freebox → DHCP, add a static lease for the TV's MAC so its IP never changes.
