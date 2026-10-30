package discovery

import (
	"encoding/json"
	"os/exec"
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
