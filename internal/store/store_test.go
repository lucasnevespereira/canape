package store

import (
	"os"
	"path/filepath"
	"testing"
)

// A file from before homes existed must come back as Legacy, not be lost.
func TestLoadKeepsPreHomesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CANAPE_DATA", dir)
	old := `{"tv":{"ip":"192.168.1.20","mac":"aa:bb:cc:dd:ee:ff","key":"k"},"wifi":{"ssid":"Net","password":"p"},"freebox":{"app_token":"t"},"dashboard":{"key":"d"}}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Legacy == nil || s.Legacy.TV.Key != "k" || s.Legacy.Wifi.SSID != "Net" || s.Legacy.Freebox.AppToken != "t" {
		t.Fatalf("legacy home not recovered: %+v", s.Legacy)
	}
	if s.Dashboard.Key != "d" || len(s.Homes) != 0 {
		t.Fatalf("dashboard=%q homes=%d", s.Dashboard.Key, len(s.Homes))
	}

	// Once saved in the new shape, the old top-level fields are gone.
	s.Homes["router"] = s.Legacy
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Legacy != nil || s.Homes["router"].TV.Key != "k" {
		t.Fatalf("after save: legacy=%v homes=%+v", s.Legacy, s.Homes)
	}
}
