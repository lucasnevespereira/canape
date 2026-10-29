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

const usage = `canape controls your TV from the terminal. The web dashboard comes later.

  canape tv pair               find the TV and pair (accept the prompt on screen)
  canape tv toast <message>    show a notification on the TV
  canape tv off                turn the TV off
  canape tv on                 turn the TV on (Wake-on-LAN)
  canape tv volume [0-100]     show or set the volume
  canape tv input [name]       list inputs, or switch to one (e.g. hdmi1, apple)
  canape tv app [name]         list apps, or launch one (e.g. youtube)
  canape tv open <url>         open a web page on the TV
  canape tv call <uri> [json]  send any SSAP request and print the reply,
                               e.g. canape tv call audio/getVolume

State is kept in %s (set CANAPE_DATA to move it).
`

func main() {
	args := os.Args[1:]
	if len(args) < 2 || args[0] != "tv" {
		fmt.Printf(usage, statePath())
		os.Exit(2)
	}
	if err := tvCommand(args[1], args[2:]); err != nil {
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
