package discovery

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Link is the Wi-Fi connection as this machine sees it. No router access
// needed, so it works on any network.
type Link struct {
	Band     string `json:"band"`     // "5 GHz"
	Width    string `json:"width"`    // "160 MHz"
	Channel  string `json:"channel"`  // "108"
	Standard string `json:"standard"` // "Wi-Fi 6"
	Security string `json:"security"` // "WPA2"
	Signal   int    `json:"signal"`   // dBm, closer to 0 is stronger
	Rate     int    `json:"rate"`     // Mbps between this machine and the box
}

// SignalWords says how good the signal is in words, with the dBm after
// them for those who read it.
func (l Link) SignalWords() string {
	dbm := fmt.Sprintf(" (%d dBm)", l.Signal)
	switch {
	case l.Signal == 0:
		return ""
	case l.Signal >= -55:
		return "excellent signal" + dbm
	case l.Signal >= -67:
		return "good signal" + dbm
	case l.Signal >= -75:
		return "fair signal" + dbm
	}
	return "weak signal" + dbm + ", move closer to the box"
}

// Details is the connection in numbers, e.g. "channel 108, 160 MHz wide,
// 2401 Mbps link to the box". Empty when macOS gave none of them.
func (l Link) Details() string {
	var parts []string
	if l.Channel != "" {
		parts = append(parts, "channel "+l.Channel)
	}
	if l.Width != "" {
		parts = append(parts, l.Width+" wide")
	}
	if l.Rate > 0 {
		parts = append(parts, fmt.Sprintf("%d Mbps link to the box", l.Rate))
	}
	return strings.Join(parts, ", ")
}

// SecurityWords grades the Wi-Fi encryption in plain words.
func (l Link) SecurityWords() string {
	switch l.Security {
	case "WPA3":
		return "WPA3 (very secure)"
	case "WPA2":
		return "WPA2 (secure)"
	case "WPA":
		return "WPA (outdated, change it in the box)"
	case "WEP":
		return "WEP (unsafe, change it in the box)"
	case "open":
		return "no password, anyone nearby can join"
	}
	return l.Security
}

var channelRe = regexp.MustCompile(`^(\d+) \((\d+)GHz(?:, (\d+)MHz)?\)`)

// parseLink turns system_profiler's fields, like "108 (5GHz, 160MHz)",
// "802.11ax" and "spairport_security_mode_wpa2_personal", into a Link.
func parseLink(channel, phy, security, signal string, rate int) *Link {
	l := &Link{Rate: rate}
	if m := channelRe.FindStringSubmatch(channel); m != nil {
		l.Channel, l.Band = m[1], m[2]+" GHz"
		if m[3] != "" {
			l.Width = m[3] + " MHz"
		}
	}
	l.Standard = map[string]string{
		"802.11be": "Wi-Fi 7", "802.11ax": "Wi-Fi 6", "802.11ac": "Wi-Fi 5", "802.11n": "Wi-Fi 4",
	}[phy]
	if l.Standard == "Wi-Fi 6" && l.Band == "6 GHz" {
		l.Standard = "Wi-Fi 6E"
	}
	sec := strings.TrimPrefix(security, "spairport_security_mode_")
	switch {
	case strings.Contains(sec, "wpa3"):
		l.Security = "WPA3"
	case strings.Contains(sec, "wpa2"):
		l.Security = "WPA2"
	case strings.Contains(sec, "wpa"):
		l.Security = "WPA"
	case strings.Contains(sec, "wep"):
		l.Security = "WEP"
	case sec == "none":
		l.Security = "open"
	}
	if dbm, _, ok := strings.Cut(signal, " dBm"); ok {
		l.Signal, _ = strconv.Atoi(strings.TrimSpace(dbm))
	}
	return l
}
