package main

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
