package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lucasnevespereira/canape/internal/discovery"
)

func scan() error {
	fmt.Println("Scanning the network (a few seconds)...")
	res, err := discovery.Scan()
	if err != nil {
		return err
	}
	fmt.Printf("\nThis machine   %s\n", res.LocalIP)
	if f := res.Freebox; f != nil {
		nick := ""
		if f.Nick != "" {
			nick = " (" + f.Nick + ")"
		}
		fmt.Printf("Freebox        %s%s, API %s\n               %s\n", f.Model, nick, f.Version, f.BaseURL)
	}

	var known, other []*discovery.Device
	for _, d := range res.Devices {
		if d.Kind() != "" || d.Name != "" || d.Alias != "" || len(d.Services) > 0 || len(d.Open) > 0 {
			known = append(known, d)
		} else {
			other = append(other, d)
		}
	}

	fmt.Printf("\nDevices that advertise themselves (%d)\n", len(known))
	for _, d := range known {
		title := firstNonEmpty(d.Name, d.Alias, d.Host, "unnamed")
		if k := d.Kind(); k != "" {
			title += "  [" + k + "]"
		}
		fmt.Printf("\n  %-15s %s\n", d.IP, title)
		line("mac", d.MAC)
		line("host", d.Host)
		model := d.Model
		if !strings.Contains(model, d.HW) {
			model += " " + d.HW
		}
		line("model", strings.TrimSpace(model))
		line("software", d.Server)
		var svcs []string
		for s := range d.Services {
			svcs = append(svcs, s)
		}
		sort.Strings(svcs)
		line("services", strings.Join(svcs, " "))
		for _, p := range d.Open {
			line("open", strings.TrimSpace(fmt.Sprintf("%d  %s", p, d.PortLabel(p))))
		}
		if d.Kind() == "LG webOS TV" {
			line("canape", "pair it with: canape tv pair")
		}
	}

	if len(other) > 0 {
		fmt.Printf("\nOther devices on the network (%d)\n\n", len(other))
		for _, d := range other {
			fmt.Printf("  %-15s %-17s  %s\n", d.IP, d.MAC, d.Host)
		}
	}
	fmt.Println()
	return nil
}

func line(label, value string) {
	if value != "" {
		fmt.Printf("                  %-9s %s\n", label, value)
	}
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
