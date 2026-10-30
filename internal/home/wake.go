package home

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/store"
	"github.com/lucasnevespereira/canape/internal/tv"
)

// Candidate is a device online right now that could be saved for waking.
type Candidate struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	MAC    string `json:"mac"`
	Vendor string `json:"vendor"`
}

// Candidates lists the devices online now, minus the ones already saved.
// A computer has to be on to be added: that's when its MAC is known.
func (h *Here) Candidates() ([]Candidate, error) {
	var list []Candidate
	if h.Freebox.AppToken != "" {
		hosts, err := h.Devices(false)
		if err != nil {
			return nil, err
		}
		for _, d := range hosts {
			list = append(list, Candidate{Name: d.Name, IP: d.IPv4(), MAC: strings.ToLower(d.MAC()), Vendor: d.Vendor})
		}
	} else {
		res, err := discovery.Scan()
		if err != nil {
			return nil, err
		}
		for _, d := range res.Devices {
			if d.MAC != "" && !d.Self {
				list = append(list, Candidate{Name: firstNonEmpty(d.Name, d.Alias, d.Host, d.IP), IP: d.IP, MAC: d.MAC})
			}
		}
	}
	var fresh []Candidate
	for _, c := range list {
		if h.computer(c.MAC) == nil && c.MAC != "" {
			fresh = append(fresh, c)
		}
	}
	return fresh, nil
}

// AddComputer saves a device for waking, under the name the user chose.
func (h *Here) AddComputer(name, mac, ip string) error {
	mac = strings.ToLower(mac)
	if name == "" || mac == "" {
		return errors.New("a computer needs a name and a MAC address")
	}
	if c := h.computer(mac); c != nil {
		c.Name, c.IP = name, ip
	} else {
		h.Computers = append(h.Computers, store.Computer{Name: name, MAC: mac, IP: ip})
	}
	return h.Save()
}

// ForgetComputer removes a saved computer, by name or MAC.
func (h *Here) ForgetComputer(nameOrMAC string) error {
	c, err := h.findComputer(nameOrMAC)
	if err != nil {
		return err
	}
	kept := h.Computers[:0]
	for _, other := range h.Computers {
		if other.MAC != c.MAC {
			kept = append(kept, other)
		}
	}
	h.Computers = kept
	return h.Save()
}

// WakeComputer sends Wake-on-LAN to a saved computer, by name or MAC.
func (h *Here) WakeComputer(nameOrMAC string) (store.Computer, error) {
	c, err := h.findComputer(nameOrMAC)
	if err != nil {
		return store.Computer{}, err
	}
	ip := c.IP
	if ip == "" {
		ip = discovery.LocalIP() // only its /24 matters, for the broadcast
	}
	return c, tv.Wake(c.MAC, ip)
}

// Online reports which saved computers answer right now, by MAC. Without a
// Freebox it can't tell, and returns nil.
func (h *Here) Online() map[string]bool {
	if h.Freebox.AppToken == "" {
		return nil
	}
	hosts, err := h.Devices(false)
	if err != nil {
		return nil
	}
	on := map[string]bool{}
	for _, d := range hosts {
		on[strings.ToLower(d.MAC())] = true
	}
	return on
}

func (h *Here) computer(mac string) *store.Computer {
	for i := range h.Computers {
		if h.Computers[i].MAC == strings.ToLower(mac) {
			return &h.Computers[i]
		}
	}
	return nil
}

func (h *Here) findComputer(nameOrMAC string) (store.Computer, error) {
	if c := h.computer(nameOrMAC); c != nil {
		return *c, nil
	}
	var choices []tv.Choice
	for _, c := range h.Computers {
		choices = append(choices, tv.Choice{ID: c.MAC, Label: c.Name})
	}
	if len(choices) == 0 {
		return store.Computer{}, errors.New("no computer saved for waking yet. Run: canape wake add")
	}
	m, err := tv.Match(nameOrMAC, choices)
	if err != nil {
		return store.Computer{}, fmt.Errorf("%w. Run canape wake to list them", err)
	}
	return *h.computer(m.ID), nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
