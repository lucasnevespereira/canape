package discovery

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// WiFi reads the current Wi-Fi link from system_profiler, which needs no
// permission. macOS hides the network's name from command-line tools
// without location access, so the name comes from elsewhere (the router or
// the saved Wi-Fi). Returns nil when not on Wi-Fi.
func WiFi() *Link {
	out, err := exec.Command("system_profiler", "SPAirPortDataType", "-json").Output()
	if err != nil {
		return nil
	}
	var data struct {
		Items []struct {
			Interfaces []struct {
				Current *struct {
					Channel  string `json:"spairport_network_channel"`
					PHY      string `json:"spairport_network_phymode"`
					Security string `json:"spairport_security_mode"`
					Signal   string `json:"spairport_signal_noise"`
					Rate     int    `json:"spairport_network_rate"`
				} `json:"spairport_current_network_information"`
			} `json:"spairport_airport_interfaces"`
		} `json:"SPAirPortDataType"`
	}
	if json.Unmarshal(out, &data) != nil {
		return nil
	}
	for _, item := range data.Items {
		for _, iface := range item.Interfaces {
			if c := iface.Current; c != nil && c.Channel != "" {
				return parseLink(c.Channel, c.PHY, c.Security, c.Signal, c.Rate)
			}
		}
	}
	return nil
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
