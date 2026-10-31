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

func TestMaker(t *testing.T) {
	for org, want := range map[string]string{
		"Espressif Inc.":                         "Espressif",
		"TP-LINK TECHNOLOGIES CO.,LTD.":          "TP-Link",
		"SAMSUNG ELECTRO-MECHANICS(THAILAND)":    "Samsung",
		"Samsung Electronics Co.,Ltd":            "Samsung",
		"LG Electronics (Mobile Communications)": "LG",
		"Shenzhen Bilian Electronic Co.，Ltd":     "Bilian",
		"Hon Hai Precision Ind. Co.,Ltd.":        "Foxconn",
		"Raspberry Pi Trading Ltd":               "Raspberry Pi",
		"Signify B.V.":                           "Philips Hue",
		"Tuya Smart Inc.":                        "Tuya",
		"Huawei Device Co., Ltd.":                "Huawei",
		"Juniper Networks":                       "Juniper",
		"IEEE Registration Authority":            "",
		"":                                       "",
	} {
		if got := maker(org); got != want {
			t.Errorf("maker(%q) = %q, want %q", org, got, want)
		}
	}
}

func TestVendor(t *testing.T) {
	if got := Vendor("b8:27:eb:00:00:01"); got != "Raspberry Pi" {
		t.Errorf("Vendor = %q", got)
	}
	// Phones pick a random private address per network: no maker.
	if !Private("3e:00:00:00:00:01") || Private("b8:27:eb:00:00:01") || Vendor("3e:00:00:00:00:01") != "" {
		t.Error("private address not detected")
	}
}

func TestKindFromPortsAndName(t *testing.T) {
	for _, c := range []struct {
		d    Device
		want string
	}{
		{Device{Open: []int{443, 8800}, Vendor: "TP-Link"}, "Camera"},
		{Device{Open: []int{8800}, Vendor: "Espressif"}, ""}, // 8800 means a camera on Tapo only
		{Device{Open: []int{554}}, "Camera"},
		{Device{Open: []int{6668}}, "Smart home device"},
		{Device{Host: "tapo-plug.home"}, "Smart plug"},
		{Device{Host: "pet-feeder.home"}, "Pet feeder"},
		{Device{Host: "iphone-de-alex.home"}, "iPhone"},
	} {
		if got := c.d.Kind(); got != c.want {
			t.Errorf("Kind(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
}
