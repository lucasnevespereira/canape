// Package home ties the saved state to live devices on the network this
// machine is on. The CLI, the dashboard and the MCP tools all go through it.
package home

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
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

// ErrUnknownNetwork means the router can't be identified, so there's no
// home to load. What needs no home, like the network report, still works.
var ErrUnknownNetwork = errors.New("can't tell which network this is")

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
		return nil, fmt.Errorf("%w: %v", ErrUnknownNetwork, err)
	}
	h, saved := st.Homes[mac], true
	if st.Legacy != nil {
		// The file predates homes: its one home belongs to this network.
		if h == nil {
			h = st.Legacy
			h.Name = "home"
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

// ErrNoTV means no TV that canape can drive answered: none, or it's off.
var ErrNoTV = errors.New("no TV that canape can control answered")

// FindTV returns the paired TV called name, matched loosely. An empty name
// means the first TV paired.
func (h *Here) FindTV(name string) (*store.TV, error) {
	if len(h.TVs) == 0 {
		return nil, fmt.Errorf("no TV paired on this network (%s). Run: canape setup", h.Name)
	}
	if name == "" {
		return &h.TVs[0], nil
	}
	var choices []tv.Choice
	for i, t := range h.TVs {
		choices = append(choices, tv.Choice{ID: strconv.Itoa(i), Label: t.Name})
	}
	c, err := tv.Match(name, choices)
	if err != nil {
		return nil, fmt.Errorf("no TV: %w. The TVs here: %s", err, h.TVNames())
	}
	i, _ := strconv.Atoi(c.ID)
	return &h.TVs[i], nil
}

// TVCommand spells a CLI command for the TV called name, leaving the name
// out while it's the only TV: "canape tv on", or "canape tv projector on".
func (h *Here) TVCommand(name, command string) string {
	if len(h.TVs) > 1 {
		return "canape tv " + name + " " + command
	}
	return "canape tv " + command
}

// TVNames lists the paired TVs, comma separated.
func (h *Here) TVNames() string {
	var names []string
	for _, t := range h.TVs {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// PairTVs looks for TVs of every brand and pairs those not saved yet,
// leaving the others alone. found is called before a TV shows its prompt,
// name once it's paired, with a free name to suggest. It returns the TVs it
// paired, after greeting each on screen.
// ponytail: one TV per brand per run, since Discover returns the first that answers. Return every IP if a home gets two TVs of a brand.
func (h *Here) PairTVs(found func(brand, ip string), prompted func(), name func(suggested string) string) ([]store.TV, error) {
	brands := make([]string, 0, len(drivers))
	for b := range drivers {
		brands = append(brands, b)
	}
	sort.Strings(brands)
	var paired []store.TV
	var errs, failed []error
	for _, brand := range brands {
		d := drivers[brand]
		ip, err := d.discover(3 * time.Second)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		mac := discovery.MAC(ip)
		if h.hasTV(brand, ip, mac) {
			continue
		}
		found(d.name, ip)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t, key, err := d.connect(ctx, ip, "", prompted)
		cancel()
		if err != nil {
			failed = append(failed, fmt.Errorf("pairing with the %s TV at %s failed: %w", d.name, ip, err))
			continue
		}
		t.Notify("canape is paired")
		t.Close()
		suggested := h.freeTVName()
		chosen := TVName(name(suggested))
		if chosen == "" || h.tvNamed(chosen) {
			chosen = suggested
		}
		saved := store.TV{Name: chosen, Brand: brand, IP: ip, MAC: mac, Key: key}
		h.TVs = append(h.TVs, saved)
		if err := h.Save(); err != nil {
			return paired, err
		}
		paired = append(paired, saved)
	}
	if len(failed) > 0 {
		return paired, errors.Join(failed...)
	}
	if len(paired) == 0 {
		return nil, fmt.Errorf("%w (%w)", ErrNoTV, errors.Join(errs...))
	}
	return paired, nil
}

func (h *Here) hasTV(brand, ip, mac string) bool {
	for _, t := range h.TVs {
		if t.Brand == brand && (t.IP == ip || mac != "" && t.MAC == mac) {
			return true
		}
	}
	return false
}

// freeTVName is "tv", then "tv-2", "tv-3"...
func (h *Here) freeTVName() string {
	name := "tv"
	for i := 2; h.tvNamed(name); i++ {
		name = fmt.Sprintf("tv-%d", i)
	}
	return name
}

// TVName turns what the user typed into a name that's easy to type again:
// "Living Room" becomes "living-room".
func TVName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), "-")
}

func (h *Here) tvNamed(name string) bool {
	for _, t := range h.TVs {
		if strings.EqualFold(t.Name, name) {
			return true
		}
	}
	return false
}

// RenameTV gives a paired TV the name the user will type.
func (h *Here) RenameTV(name, to string) error {
	t, err := h.FindTV(name)
	if err != nil {
		return err
	}
	to = TVName(to)
	if to == "" {
		return errors.New("give the TV a name, like projector or bedroom")
	}
	if !strings.EqualFold(t.Name, to) && h.tvNamed(to) {
		return fmt.Errorf("another TV is already called %q", to)
	}
	t.Name = to
	return h.Save()
}

// ForgetTV removes a paired TV from this home.
func (h *Here) ForgetTV(name string) (string, error) {
	t, err := h.FindTV(name)
	if err != nil {
		return "", err
	}
	gone := t.Name
	h.TVs = slices.DeleteFunc(h.TVs, func(x store.TV) bool { return x.Name == gone })
	return gone, h.Save()
}

// ConnectTV dials the TV called name ("" for the first) and authenticates
// with the saved key. If the TV forgot canape, it shows the accept prompt
// again and the new key is saved. If the TV moved to another IP, it's found
// again and the new IP is saved.
func (h *Here) ConnectTV(name string) (tv.TV, error) {
	if name == "" && len(h.TVs) > 1 {
		return h.connectFirstOn()
	}
	saved, err := h.FindTV(name)
	if err != nil {
		return nil, err
	}
	d, ok := drivers[saved.Brand]
	if !ok {
		return nil, fmt.Errorf("this canape doesn't support %q TVs", saved.Brand)
	}
	prompted := func() {
		// stderr: stdout carries the protocol under `canape mcp`.
		fmt.Fprintf(os.Stderr, "The %s TV forgot canape. Accept the prompt on screen...\n", saved.Name)
	}
	ip := saved.IP
	t, key, err := dialTV(d, ip, saved.Key, prompted)
	if err != nil && saved.MAC != "" {
		// The router may have given the TV a new IP. Look for it, and only
		// trust a TV with the MAC saved at pairing, not the neighbour's.
		if found, derr := d.discover(1500 * time.Millisecond); derr == nil && found != ip && discovery.MAC(found) == saved.MAC {
			ip = found
			t, key, err = dialTV(d, ip, saved.Key, prompted)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%w\nIs the TV on? Turn it on with: %s", err, h.TVCommand(saved.Name, "on"))
	}
	if key != saved.Key || ip != saved.IP {
		saved.Key, saved.IP = key, ip
		if err := h.Save(); err != nil {
			t.Close()
			return nil, err
		}
	}
	return t, nil
}

// connectFirstOn is for a command that names no TV in a home with several:
// it goes to the one that's on, since people rarely watch two at once.
func (h *Here) connectFirstOn() (tv.TV, error) {
	var errs []error
	for _, saved := range h.TVs {
		t, err := h.ConnectTV(saved.Name)
		if err == nil {
			return t, nil
		}
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("none of the TVs answered (%s). Turn one on with: canape tv <name> on\n%w", h.TVNames(), errors.Join(errs...))
}

// dialTV gives the TV 2 seconds to answer. On a home network it takes well
// under half a second, so a longer wait only makes "the TV is off" slow.
func dialTV(d driver, ip, key string, prompted func()) (tv.TV, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return d.connect(ctx, ip, key, prompted)
}

// WakeTV turns the TV called name on with Wake-on-LAN. A TV that's off
// can't be dialed.
func (h *Here) WakeTV(name string) error {
	t, err := h.FindTV(name)
	if err != nil {
		return err
	}
	if t.MAC == "" {
		return fmt.Errorf("canape doesn't know the %s TV's network address. Run canape tv pair with the TV on", t.Name)
	}
	return tv.Wake(t.MAC, t.IP)
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
	// The page reads both as lists: a TV without inputs sends [], not null.
	if st.Inputs == nil {
		st.Inputs = []tv.Input{}
	}
	if st.Apps == nil {
		st.Apps = []tv.App{}
	}
	return st, err
}

// Devices lists the hosts the router knows, online ones only unless all.
func (h *Here) Devices(all bool) ([]freebox.Host, error) {
	if h.Freebox.AppToken == "" {
		return nil, errors.New("the router's device list needs a paired Freebox (canape setup pairs one). canape scan works on any network")
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
