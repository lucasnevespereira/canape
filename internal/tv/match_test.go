package tv

import "testing"

func TestMatch(t *testing.T) {
	choices := []Choice{
		{"HDMI_1", "Apple"},
		{"HDMI_2", "PS5"},
		{"youtube.leanback.v4", "YouTube"},
		{"amazon", "Prime Video"},
		{"com.apple.appletv", "Apple TV"},
	}
	for query, want := range map[string]string{
		"hdmi1":    "HDMI_1",
		"HDMI_1":   "HDMI_1",
		"apple":    "HDMI_1", // exact label wins over the partial match on Apple TV
		"youtube":  "youtube.leanback.v4",
		"prime":    "amazon",
		"apple tv": "com.apple.appletv",
	} {
		got, err := Match(query, choices)
		if err != nil || got.ID != want {
			t.Errorf("Match(%q) = %q, %v; want %q", query, got.ID, err, want)
		}
	}
	if _, err := Match("hdmi", choices); err == nil {
		t.Error(`Match("hdmi") should be ambiguous`)
	}
	if _, err := Match("netflix", choices); err == nil {
		t.Error(`Match("netflix") should match nothing`)
	}
}
