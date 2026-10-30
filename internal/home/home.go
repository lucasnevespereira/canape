// Package home ties the saved state to live devices on the network this
// machine is on. The CLI, the dashboard and the MCP tools all go through it.
package home

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/router/freebox"
	"github.com/lucasnevespereira/canape/internal/store"
	"github.com/lucasnevespereira/canape/internal/tv/lg"
)

// Here is the home for the current network. Its fields (TV, Wifi, Freebox)
// read and write that home's entry in the state file.
type Here struct {
	*store.Home
	Router string // router MAC, the home's key
	State  *store.State
	Saved  bool // false for a network canape hasn't set up yet
}

// Load finds the current network's home. A new network gets an empty home
// that's only written once something is paired, so read-only commands
// never add entries.
func Load() (*Here, error) {
	st, err := store.Load()
	if err != nil {
		return nil, err
	}
	_, mac, err := discovery.Router()
	if err != nil {
		return nil, err
	}
	h, saved := st.Homes[mac], true
	if st.Legacy != nil {
		// The file predates homes: its one home belongs to this network.
		if h == nil {
			h = st.Legacy
			h.Name = "home"
			if h.TV.Key != "" && h.TV.Brand == "" {
				h.TV.Brand = "lg" // the only brand before homes
			}
			st.Homes[mac] = h
		}
		st.Legacy = nil
		if err := st.Save(); err != nil {
			return nil, err
		}
	}
	if h == nil {
		h, saved = &store.Home{Name: newName(st)}, false
	}
	return &Here{Home: h, Router: mac, State: st, Saved: saved}, nil
}

// newName picks "home" for the first network, then "home-2", "home-3"...
// until the user renames it.
func newName(st *store.State) string {
	taken := map[string]bool{}
	for _, h := range st.Homes {
		taken[h.Name] = true
	}
	name := "home"
	for i := 2; taken[name]; i++ {
		name = fmt.Sprintf("home-%d", i)
	}
	return name
}

// Save writes this home, adding it to the state file if it's new.
func (h *Here) Save() error {
	h.State.Homes[h.Router] = h.Home
	h.Saved = true
	return h.State.Save()
}

// ConnectTV dials the paired TV and authenticates with the saved key. If the
// TV forgot canape, it shows the accept prompt again and the new key is saved.
func (h *Here) ConnectTV() (*lg.TV, error) {
	if h.TV.Key == "" {
		return nil, fmt.Errorf("no TV paired on this network (%s). Run: canape tv pair", h.Name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tv, err := lg.Dial(ctx, h.TV.IP)
	if err != nil {
		return nil, fmt.Errorf("%w\nIs the TV on? If its IP changed, run: canape tv pair", err)
	}
	key, err := tv.Register(h.TV.Key, func() {
		// stderr: stdout carries the protocol under `canape mcp`.
		fmt.Fprintln(os.Stderr, "The TV forgot canape. Accept the prompt on screen...")
	})
	if err != nil {
		tv.Close()
		return nil, err
	}
	if key != h.TV.Key {
		h.TV.Key = key
		if err := h.Save(); err != nil {
			tv.Close()
			return nil, err
		}
	}
	return tv, nil
}

// WakeTV turns the TV on with Wake-on-LAN; a TV that's off can't be dialed.
func (h *Here) WakeTV() error {
	if h.TV.MAC == "" {
		return fmt.Errorf("no TV paired on this network (%s). Run: canape tv pair (with the TV on)", h.Name)
	}
	return lg.Wake(h.TV.MAC, h.TV.IP)
}

type TVStatus struct {
	On      bool       `json:"on"`
	Volume  int        `json:"volume"`
	Muted   bool       `json:"muted"`
	Current string     `json:"current"` // app on screen; inputs count as apps
	Inputs  []lg.Input `json:"inputs"`
	Apps    []lg.App   `json:"apps"`
}

// Status reads what the dashboard and the tv_status MCP tool show.
func Status(tv *lg.TV) (TVStatus, error) {
	st := TVStatus{On: true}
	v, err := tv.Volume()
	if err != nil {
		return st, err
	}
	st.Volume, st.Muted = v.Level, v.Muted
	st.Current, _ = tv.ForegroundApp()
	inputs, err := tv.Inputs()
	if err != nil {
		return st, err
	}
	for _, in := range inputs {
		if strings.HasPrefix(in.ID, "HDMI") { // AV and component inputs are legacy noise
			st.Inputs = append(st.Inputs, in)
		}
	}
	st.Apps, err = tv.Apps()
	return st, err
}

// Devices lists the hosts the router knows, online ones only unless all.
func (h *Here) Devices(all bool) ([]freebox.Host, error) {
	if h.Freebox.AppToken == "" {
		return nil, errors.New("canape isn't paired with this network's router. On a Freebox, run: canape freebox pair")
	}
	fbx, err := freebox.New(h.Freebox.AppToken)
	if err != nil {
		return nil, err
	}
	hosts, err := fbx.Hosts()
	if err != nil || all {
		return hosts, err
	}
	var online []freebox.Host
	for _, host := range hosts {
		if host.Reachable {
			online = append(online, host)
		}
	}
	return online, nil
}
