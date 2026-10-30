package store

import (
	"os"
	"path/filepath"
	"testing"
)

// A file from before homes existed must come back as Legacy, not be lost.
func TestLoadKeepsPreHomesFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := filepath.Dir(Path())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
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

// A state.json in the old OS config dir moves to ~/.canape on first load.
func TestLoadMovesOldLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	old := oldPath()
	if err := os.MkdirAll(filepath.Dir(old), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte(`{"homes":{"r":{"name":"home"}},"dashboard":{"key":"d"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Homes["r"] == nil || s.Dashboard.Key != "d" {
		t.Fatalf("state not carried over: %+v", s)
	}
	if Path() != filepath.Join(home, ".canape", "state.json") {
		t.Fatalf("Path() = %s", Path())
	}
	if _, err := os.Stat(Path()); err != nil {
		t.Fatalf("new file missing: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old file still there: %v", err)
	}
}
