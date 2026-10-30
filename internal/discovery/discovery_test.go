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
