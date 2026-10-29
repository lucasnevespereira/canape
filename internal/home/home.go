// Package home ties the saved state to live devices: the paired TV and the
// router. The CLI, the dashboard and the MCP tools all go through it.
package home

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/internal/router/freebox"
	"github.com/lucasnevespereira/canape/internal/store"
	"github.com/lucasnevespereira/canape/internal/tv/lg"
)

// ConnectTV dials the paired TV and authenticates with the saved key. If the
// TV forgot canape, it shows the accept prompt again and the new key is saved.
func ConnectTV(st *store.State) (*lg.TV, error) {
	if st.TV.Key == "" {
		return nil, errors.New("not paired yet. Run: canape tv pair")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tv, err := lg.Dial(ctx, st.TV.IP)
	if err != nil {
		return nil, fmt.Errorf("%w\nIs the TV on? If its IP changed, run: canape tv pair", err)
	}
	key, err := tv.Register(st.TV.Key, func() {
		// stderr: stdout carries the protocol under `canape mcp`.
		fmt.Fprintln(os.Stderr, "The TV forgot canape. Accept the prompt on screen...")
	})
	if err != nil {
		tv.Close()
		return nil, err
	}
	if key != st.TV.Key {
		st.TV.Key = key
		if err := st.Save(); err != nil {
			tv.Close()
			return nil, err
		}
	}
	return tv, nil
}

// WakeTV turns the TV on with Wake-on-LAN; a TV that's off can't be dialed.
func WakeTV(st *store.State) error {
	if st.TV.MAC == "" {
		return errors.New("no MAC address saved for the TV. Run: canape tv pair (with the TV on)")
	}
	return lg.Wake(st.TV.MAC, st.TV.IP)
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
func Devices(st *store.State, all bool) ([]freebox.Host, error) {
	if st.Freebox.AppToken == "" {
		return nil, errors.New("not paired with the Freebox yet. Run: canape freebox pair")
	}
	fbx, err := freebox.New(st.Freebox.AppToken)
	if err != nil {
		return nil, err
	}
	hosts, err := fbx.Hosts()
	if err != nil || all {
		return hosts, err
	}
	var online []freebox.Host
	for _, h := range hosts {
		if h.Reachable {
			online = append(online, h)
		}
	}
	return online, nil
}
