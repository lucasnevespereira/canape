package discovery

import "testing"

func TestPadMAC(t *testing.T) {
	if got := padMAC("AA:bb:cc:d:ee:f"); got != "aa:bb:cc:0d:ee:0f" {
		t.Errorf("padMAC = %q", got)
	}
}

func TestShortST(t *testing.T) {
	for st, want := range map[string]string{
		"urn:lge-com:service:webos-second-screen:1":   "webos-second-screen",
		"urn:schemas-upnp-org:device:MediaRenderer:1": "MediaRenderer",
		"upnp:rootdevice": "",
		"uuid:1234abcd":   "",
	} {
		if got := shortST(st); got != want {
			t.Errorf("shortST(%q) = %q, want %q", st, got, want)
		}
	}
}

func TestKindTVBrands(t *testing.T) {
	for _, c := range []struct {
		d    Device
		want string
	}{
		{Device{Model: "Samsung Electronics UE55TU7105", Open: []int{8002}}, "Samsung TV"},
		{Device{Model: "Samsung Electronics Galaxy S23"}, ""}, // a Samsung phone isn't a TV
		{Device{Services: map[string]bool{"ssdp:ScalarWebAPI": true}}, "Sony TV"},
		{Device{Services: map[string]bool{"ssdp:webos-second-screen": true}}, "LG webOS TV"},
	} {
		if got := c.d.Kind(); got != c.want {
			t.Errorf("Kind(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestKindEverydayDevices(t *testing.T) {
	for _, c := range []struct {
		d    Device
		want string
	}{
		{Device{Host: "iphone-de-lucas.home"}, "iPhone"},
		{Device{HW: "Mac16,1", Services: map[string]bool{"mdns:airplay": true}}, "Mac"},
		{Device{Services: map[string]bool{"mdns:ipp": true}}, "Printer"},
		{Device{Services: map[string]bool{"mdns:spotify-connect": true}}, "Speaker"},
		{Device{Services: map[string]bool{"mdns:airplay": true}}, "AirPlay device"},
	} {
		if got := c.d.Kind(); got != c.want {
			t.Errorf("Kind(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestParseLink(t *testing.T) {
	l := parseLink("108 (5GHz, 160MHz)", "802.11ax", "spairport_security_mode_wpa2_personal", "-40 dBm / -93 dBm", 2401)
	if l.Band != "5 GHz" || l.Width != "160 MHz" || l.Channel != "108" || l.Standard != "Wi-Fi 6" || l.Security != "WPA2" || l.Signal != -40 {
		t.Fatalf("parseLink = %+v", l)
	}
	if l.SignalWords() != "excellent signal (-40 dBm)" || l.SecurityWords() != "WPA2 (secure)" {
		t.Fatalf("words: %q %q", l.SignalWords(), l.SecurityWords())
	}
	if d := l.Details(); d != "channel 108, 160 MHz wide, 2401 Mbps link to the box" {
		t.Fatalf("details: %q", d)
	}
}
