package home

import (
	"testing"

	"github.com/lucasnevespereira/canape/internal/store"
)

func TestFindTV(t *testing.T) {
	h := &Here{Home: &store.Home{TVs: []store.TV{{Name: "tv"}, {Name: "tv-2"}, {Name: "projector"}}}}
	for name, want := range map[string]string{"": "tv", "tv": "tv", "TV-2": "tv-2", "proj": "projector"} {
		got, err := h.FindTV(name)
		if err != nil || got.Name != want {
			t.Errorf("FindTV(%q) = %v, %v, want %s", name, got, err, want)
		}
	}
	if _, err := h.FindTV("bedroom"); err == nil {
		t.Error("FindTV(bedroom) should fail")
	}
	if got := h.freeTVName(); got != "tv-3" {
		t.Errorf("freeTVName = %s", got)
	}
	if got := TVName("  Living Room "); got != "living-room" {
		t.Errorf("TVName = %s", got)
	}
}
