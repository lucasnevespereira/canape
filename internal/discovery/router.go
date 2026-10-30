package discovery

import (
	"errors"
	"net"
	"os/exec"
	"regexp"
	"strings"
)

// Router returns the default gateway's IP and MAC. The MAC identifies a home
// network: it's stable, unlike IPs, and needs no permission to read, unlike
// the Wi-Fi name, which recent macOS hides from command-line tools.
func Router() (ip, mac string, err error) {
	ip = gateway()
	if ip == "" {
		return "", "", errors.New("no router found. Is this machine on a network?")
	}
	if mac = MAC(ip); mac == "" {
		// An empty ARP cache: send a packet to fill it, then look again.
		if c, err := net.Dial("udp", net.JoinHostPort(ip, "9")); err == nil {
			c.Write([]byte{0})
			c.Close()
		}
		mac = MAC(ip)
	}
	if mac == "" {
		return ip, "", errors.New("couldn't read the router's MAC address from the ARP table")
	}
	return ip, mac, nil
}

var gatewayRe = regexp.MustCompile(`(?:gateway: |default via )(\d+\.\d+\.\d+\.\d+)`)

func gateway() string {
	// macOS prints "gateway: 192.168.1.254", Linux "default via 192.168.1.254 dev ...".
	for _, cmd := range [][]string{{"route", "-n", "get", "default"}, {"ip", "route", "show", "default"}} {
		out, err := exec.Command(cmd[0], cmd[1:]...).Output()
		if m := gatewayRe.FindStringSubmatch(string(out)); err == nil && m != nil {
			return m[1]
		}
	}
	return ""
}

var macRe = regexp.MustCompile(`([0-9a-fA-F]{1,2}:){5}[0-9a-fA-F]{1,2}`)

// MAC reads a device's MAC from the ARP cache, which is warm right after
// talking to it. Empty if not found.
func MAC(ip string) string {
	for _, cmd := range [][]string{{"arp", "-n", ip}, {"ip", "neigh", "show", ip}} {
		out, err := exec.Command(cmd[0], cmd[1:]...).Output()
		if m := macRe.FindString(string(out)); err == nil && m != "" {
			return padMAC(strings.ToLower(m))
		}
	}
	return ""
}
