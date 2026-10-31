package cmd

import (
	"strings"
	"testing"

	"github.com/lucasnevespereira/canape/internal/discovery"
)

func TestReadLinesKeepsLastLine(t *testing.T) {
	in := readLines(strings.NewReader("home\npass word"))
	if got := prompt(in, "name", ""); got != "home" {
		t.Fatalf("first line: %q", got)
	}
	if got := prompt(in, "password", ""); got != "pass word" {
		t.Fatalf("last line without a newline: %q", got)
	}
	if got := prompt(in, "after the end", "default"); got != "default" {
		t.Fatalf("after EOF: %q", got)
	}
}

func TestDeviceNameUsesMaker(t *testing.T) {
	for _, c := range []struct {
		d    discovery.Device
		want string
	}{
		{discovery.Device{Host: "lwip0.home", Vendor: "Espressif"}, "Espressif device"},
		{discovery.Device{Host: "c210.home", Vendor: "TP-Link"}, "TP-Link c210"},
		{discovery.Device{Host: "pet-feeder.home", Vendor: "Tuya"}, "pet-feeder"},
		{discovery.Device{Host: "tapo-plug.home", Vendor: "TP-Link"}, "tapo-plug"},
		{discovery.Device{Host: "android.home", Alias: "SpotifyConnect", Vendor: "Amazon"}, "Amazon device"},
		{discovery.Device{IP: "192.168.1.20"}, "192.168.1.20"},
	} {
		if got := deviceName(&c.d); got != c.want {
			t.Errorf("deviceName(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestDeviceNameSkipsRandomIDs(t *testing.T) {
	d := &discovery.Device{IP: "192.168.1.20", Alias: "89BDF4CE-C879-4487-825E-3A77793065E8", Host: "iphone-de-alex.home"}
	if got := deviceName(d); got != "iphone-de-alex" {
		t.Fatalf("got %q", got)
	}
}
