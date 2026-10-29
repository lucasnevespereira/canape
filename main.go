// Command canape controls the home from the couch.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const usage = `canape controls your TV from the couch.

  canape serve [addr]          run the dashboard (default :8080)

  canape tv pair              find the TV and pair (accept the prompt on screen)
  canape tv toast <message>    show a notification on the TV
  canape tv off                turn the TV off
  canape tv on                 turn the TV on (Wake-on-LAN)
  canape tv volume [0-100]     show or set the volume
  canape tv input [name]       list inputs, or switch to one (e.g. hdmi1, apple)
  canape tv app [name]         list apps, or launch one (e.g. youtube)
  canape tv open <url>         open a web page on the TV
  canape tv button <key>       press a remote key: home, back, up, down, left, right, enter
  canape tv type [text]        type into the focused text field (prompts if no text)
  canape tv install-app        install a Wi-Fi QR app on the TV (needs Developer Mode)
  canape tv call <uri> [json]  send any SSAP request and print the reply,
                               e.g. canape tv call audio/getVolume

  canape wifi set              save the Wi-Fi name and password (name comes from the Freebox)
  canape wifi                  show the Wi-Fi QR code in the terminal
  canape wifi tvapp [dir]      write the TV app files only (install-app does it all)

  canape freebox pair         authorize canape on the Freebox (confirm on its screen)
  canape freebox devices [all] devices online now, or every device seen

State is kept in %s (set CANAPE_DATA to move it).
`

func main() {
	args := os.Args[1:]
	var err error
	switch {
	case len(args) >= 1 && args[0] == "serve":
		addr := ":8080"
		if len(args) > 1 {
			addr = args[1]
		}
		err = serve(addr)
	case len(args) >= 2 && args[0] == "tv":
		err = tvCommand(args[1], args[2:])
	case len(args) >= 1 && args[0] == "wifi":
		err = wifiCommand(args[1:])
	case len(args) >= 2 && args[0] == "freebox":
		err = freeboxCommand(args[1], args[2:])
	default:
		fmt.Printf(usage, statePath())
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "canape:", err)
		os.Exit(1)
	}
}

type state struct {
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

func statePath() string {
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

func loadState() (*state, error) {
	s := &state{}
	b, err := os.ReadFile(statePath())
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, s)
}

// saveState writes owner-only: the file holds the TV's client key.
func saveState(s *state) error {
	p := statePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}
