// Package store keeps canape's state in one JSON file: one entry per home
// network (paired TV, router, Wi-Fi to share) and the dashboard login key.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type TV struct {
	Brand string `json:"brand"` // driver name, e.g. "lg"
	IP    string `json:"ip"`
	MAC   string `json:"mac"` // for Wake-on-LAN
	Key   string `json:"key"` // pairing key the TV handed out
}

type Wifi struct {
	SSID     string `json:"ssid"`
	Password string `json:"password"`
}

type Freebox struct {
	AppToken string `json:"app_token"`
}

// Home is everything canape knows about one network.
type Home struct {
	Name    string  `json:"name"`
	TV      TV      `json:"tv"`
	Wifi    Wifi    `json:"wifi"`
	Freebox Freebox `json:"freebox"`
}

type State struct {
	// Homes are keyed by their router's MAC address, which identifies the
	// network without needing its name.
	Homes     map[string]*Home `json:"homes"`
	Dashboard struct {
		Key string `json:"key"` // in the login link. A device that opened it once keeps it as a cookie
	} `json:"dashboard"`

	// Legacy is the single home the file held before homes existed. It can
	// only be filed under a router once the network is known, so the caller
	// moves it into Homes.
	Legacy *Home `json:"-"`
}

// Path is ~/.canape/state.json, the same on every system, or
// $CANAPE_DATA/state.json when set (Docker, tests).
func Path() string {
	if dir := os.Getenv("CANAPE_DATA"); dir != "" {
		return filepath.Join(dir, "state.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".canape", "state.json")
}

// oldPath is where state.json lived before ~/.canape: the OS config dir.
func oldPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfg, "canape", "state.json")
}

func Load() (*State, error) {
	s := &State{Homes: map[string]*Home{}}
	b, err := os.ReadFile(Path())
	if errors.Is(err, fs.ErrNotExist) && os.Getenv("CANAPE_DATA") == "" {
		b, err = moveOld()
	}
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Homes == nil {
		s.Homes = map[string]*Home{}
	}
	var old Home
	if json.Unmarshal(b, &old) == nil && (old.TV.Key != "" || old.Wifi.SSID != "" || old.Freebox.AppToken != "") {
		s.Legacy = &old
	}
	return s, nil
}

// moveOld moves a state.json from the old location into ~/.canape and
// returns its contents. fs.ErrNotExist means there was none.
func moveOld() ([]byte, error) {
	old := oldPath()
	if old == "" {
		return nil, fs.ErrNotExist
	}
	b, err := os.ReadFile(old)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(Path(), b, 0o600); err != nil {
		return nil, err
	}
	os.Remove(old)
	os.Remove(filepath.Dir(old)) // only succeeds if the folder is now empty
	return b, nil
}

// Save writes owner-only: the file holds pairing keys and Wi-Fi passwords.
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
