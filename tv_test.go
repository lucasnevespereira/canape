package main

import "testing"

func TestPick(t *testing.T) {
	choices := []choice{
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
		got, err := pick(query, choices)
		if err != nil || got.id != want {
			t.Errorf("pick(%q) = %q, %v; want %q", query, got.id, err, want)
		}
	}
	if _, err := pick("hdmi", choices); err == nil {
		t.Error(`pick("hdmi") should be ambiguous`)
	}
	if _, err := pick("netflix", choices); err == nil {
		t.Error(`pick("netflix") should match nothing`)
	}
}
