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

func TestDeviceNameSkipsRandomIDs(t *testing.T) {
	d := &discovery.Device{IP: "192.168.1.20", Alias: "89BDF4CE-C879-4487-825E-3A77793065E8", Host: "iphone-de-alex.home"}
	if got := deviceName(d); got != "iphone-de-alex" {
		t.Fatalf("got %q", got)
	}
}
