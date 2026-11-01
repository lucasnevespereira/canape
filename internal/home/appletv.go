package home

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/lucasnevespereira/canape/internal/appletv"
	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/store"
)

// FindAppleTVs scans the network for Apple TVs that take remote commands.
func FindAppleTVs() ([]*store.AppleTV, error) {
	res, err := discovery.Scan()
	if err != nil {
		return nil, err
	}
	var found []*store.AppleTV
	for _, d := range res.Devices {
		if d.Kind() == "Apple TV" && d.CompanionPort > 0 {
			found = append(found, &store.AppleTV{Name: d.Alias, IP: d.IP, Port: d.CompanionPort})
		}
	}
	return found, nil
}

// ConnectAppleTV connects to the paired Apple TV. When its saved address
// stops answering, it finds it again by name and saves the new address.
func (h *Here) ConnectAppleTV() (*appletv.Client, error) {
	a := h.AppleTV
	if a == nil {
		return nil, fmt.Errorf("no Apple TV paired on this network (%s). Run: canape appletv pair", h.Name)
	}
	c, err := appletv.Connect(net.JoinHostPort(a.IP, strconv.Itoa(a.Port)), &a.Pairing)
	var unreachable *net.OpError // refused or timed out: the address moved, or it's asleep
	if !errors.As(err, &unreachable) {
		return c, err
	}
	found, _ := FindAppleTVs()
	for _, f := range found {
		if f.Name == a.Name {
			a.IP, a.Port = f.IP, f.Port
			if c, err = appletv.Connect(net.JoinHostPort(a.IP, strconv.Itoa(a.Port)), &a.Pairing); err == nil {
				h.Save()
			}
			return c, err
		}
	}
	return nil, fmt.Errorf("the Apple TV %q doesn't answer. Is it on and on this Wi-Fi?", a.Name)
}
