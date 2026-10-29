// Package store keeps canape's state in one JSON file: pairing keys, the
// Wi-Fi to share and the dashboard login key.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type State struct {
	TV struct {
		IP  string `json:"ip"`
		MAC string `json:"mac"`
		Key string `json:"key"`
	} `json:"tv"`
	Wifi struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	} `json:"wifi"`
	Freebox struct {
		AppToken string `json:"app_token"`
	} `json:"freebox"`
	Dashboard struct {
		Key string `json:"key"` // in the login link; a device that opened it once keeps it as a cookie
	} `json:"dashboard"`
}

// Path is $CANAPE_DATA/state.json, or the OS config dir, so the CLI works
// from any directory.
func Path() string {
	dir := os.Getenv("CANAPE_DATA")
	if dir == "" {
		cfg, err := os.UserConfigDir()
		if err != nil {
			cfg = "."
		}
		dir = filepath.Join(cfg, "canape")
	}
	return filepath.Join(dir, "state.json")
}

func Load() (*State, error) {
	s := &State{}
	b, err := os.ReadFile(Path())
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, s)
}

// Save writes owner-only: the file holds pairing keys and the Wi-Fi password.
func (s *State) Save() error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}
