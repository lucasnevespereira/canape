package version

import (
	"strings"
	"testing"
)

func TestStampedVersionWins(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	Version = "1.2.3"
	if Get() != "1.2.3" || !strings.HasPrefix(String(), "canape 1.2.3") {
		t.Fatalf("Get() = %q, String() = %q", Get(), String())
	}
}

func TestTagPattern(t *testing.T) {
	for v, want := range map[string]bool{
		"v0.1.0":        true,
		"v1.2.3-beta.1": true,
		"v0.0.0-20261030164101-791ca02f45c6+dirty": false,
		"(devel)": false,
		"":        false,
	} {
		if tag.MatchString(v) != want {
			t.Errorf("tag.MatchString(%q) = %v, want %v", v, !want, want)
		}
	}
}
