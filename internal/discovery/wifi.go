package discovery

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

// SignalWords says how good the signal is, for people who don't read dBm.
func (l Link) SignalWords() string {
	switch {
	case l.Signal == 0:
		return ""
	case l.Signal >= -55:
		return "excellent signal"
	case l.Signal >= -67:
		return "good signal"
	case l.Signal >= -75:
		return "fair signal"
	}
	return "weak signal, move closer to the box"
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
