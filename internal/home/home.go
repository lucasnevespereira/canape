// Package home ties the saved state to live devices on the network this
// machine is on. The CLI, the dashboard and the MCP tools all go through it.
package home

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/router/freebox"
	"github.com/lucasnevespereira/canape/internal/store"
	"github.com/lucasnevespereira/canape/internal/tv"
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

// PairTV looks for a TV on this network, asks it to accept canape and
// saves it in this home. found is called before the TV shows its prompt.
func (h *Here) PairTV(found func(brand, ip string), prompted func()) (tv.TV, error) {
	brands := make([]string, 0, len(drivers))
	for b := range drivers {
		brands = append(brands, b)
	}
	sort.Strings(brands)
	var errs []error
	for _, brand := range brands {
		d := drivers[brand]
		ip, err := d.discover(3 * time.Second)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		found(d.name, ip)
		key := ""
		if h.TV.Brand == brand {
			key = h.TV.Key // re-pairing the same TV skips the prompt
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t, key, err := d.connect(ctx, ip, key, prompted)
		cancel()
		if err != nil {
			return nil, err
		}
		h.TV = store.TV{Brand: brand, IP: ip, MAC: discovery.MAC(ip), Key: key}
		if err := h.Save(); err != nil {
			t.Close()
			return nil, err
		}
		return t, nil
	}
	return nil, fmt.Errorf("no supported TV answered (%w)", errors.Join(errs...))
}

// ConnectTV dials the paired TV and authenticates with the saved key. If the
// TV forgot canape, it shows the accept prompt again and the new key is saved.
// If the TV moved to another IP, it's found again and the new IP is saved.
func (h *Here) ConnectTV() (tv.TV, error) {
	if h.TV.Key == "" {
		return nil, fmt.Errorf("no TV paired on this network (%s). Run: canape setup", h.Name)
	}
	d, ok := drivers[h.TV.Brand]
	if !ok {
		return nil, fmt.Errorf("this canape doesn't support %q TVs", h.TV.Brand)
	}
	prompted := func() {
		// stderr: stdout carries the protocol under `canape mcp`.
		fmt.Fprintln(os.Stderr, "The TV forgot canape. Accept the prompt on screen...")
	}
	ip := h.TV.IP
	t, key, err := dialTV(d, ip, h.TV.Key, prompted)
	if err != nil && h.TV.MAC != "" {
		// The router may have given the TV a new IP. Look for it, and only
		// trust a TV with the MAC saved at pairing, not the neighbour's.
		if found, derr := d.discover(1500 * time.Millisecond); derr == nil && found != ip && discovery.MAC(found) == h.TV.MAC {
			ip = found
			t, key, err = dialTV(d, ip, h.TV.Key, prompted)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%w\nIs the TV on? Turn it on with: canape tv on", err)
	}
	if key != h.TV.Key || ip != h.TV.IP {
		h.TV.Key, h.TV.IP = key, ip
		if err := h.Save(); err != nil {
			t.Close()
			return nil, err
		}
	}
	return t, nil
}

// dialTV gives the TV 2 seconds to answer. On a home network it takes well
// under half a second, so a longer wait only makes "the TV is off" slow.
func dialTV(d driver, ip, key string, prompted func()) (tv.TV, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return d.connect(ctx, ip, key, prompted)
}

// WakeTV turns the TV on with Wake-on-LAN. A TV that's off can't be dialed.
func (h *Here) WakeTV() error {
	if h.TV.MAC == "" {
		return fmt.Errorf("no TV paired on this network (%s). Run: canape setup (with the TV on)", h.Name)
	}
	return tv.Wake(h.TV.MAC, h.TV.IP)
}

type TVStatus struct {
	On      bool       `json:"on"`
	Volume  int        `json:"volume"`
	Muted   bool       `json:"muted"`
	Current string     `json:"current"` // ID of the input or app on screen
	Inputs  []tv.Input `json:"inputs"`
	Apps    []tv.App   `json:"apps"`
}

// Status reads what the dashboard and the tv_status MCP tool show.
func Status(t tv.TV) (TVStatus, error) {
	st := TVStatus{On: true}
	v, err := t.Volume()
	if err != nil {
		return st, err
	}
	st.Volume, st.Muted = v.Level, v.Muted
	st.Current, _ = t.Current()
	if st.Inputs, err = t.Inputs(); err != nil {
		return st, err
	}
	st.Apps, err = t.Apps()
	return st, err
}

// Devices lists the hosts the router knows, online ones only unless all.
func (h *Here) Devices(all bool) ([]freebox.Host, error) {
	if h.Freebox.AppToken == "" {
		return nil, errors.New("canape isn't paired with this network's router. Run: canape setup")
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
