package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/router/freebox"
)

// Device kinds, grouped the way a non-expert thinks about them.
var groups = []struct{ name string }{{"TVs"}, {"Streaming"}, {"Speakers"}, {"Phones"}, {"Computers"}, {"Printers"}, {"Other"}}

var kindGroup = map[string]string{
	"LG webOS TV": "TVs", "Samsung TV": "TVs", "Sony TV": "TVs",
	"Apple TV": "Streaming", "Android TV / Google Cast": "Streaming", "Freebox Player": "Streaming",
	"Speaker": "Speakers", "AirPlay device": "Speakers",
	"iPhone": "Phones", "iPad": "Phones",
	"Mac": "Computers", "this machine": "Computers",
	"Printer": "Printers",
}

// report is what bare `canape` prints: the network in plain words, then what
// you can do on it. It works on any network, with or without router access.
func report() error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	fmt.Println("Looking around this network (a few seconds)...")

	var (
		wg      sync.WaitGroup
		link    *discovery.Link
		inet    discovery.Internet
		scan    *discovery.Result
		scanErr error
		tvState string
	)
	wg.Add(4)
	go func() { defer wg.Done(); link = discovery.WiFi() }()
	go func() { defer wg.Done(); inet = discovery.CheckInternet() }()
	go func() { defer wg.Done(); scan, scanErr = discovery.Scan() }()
	go func() { defer wg.Done(); tvState = pairedTVState(h) }()
	wg.Wait()
	if scanErr != nil {
		return scanErr
	}

	if h.Saved {
		fmt.Printf("\ncanape · %s\n\n", h.Name)
	} else {
		fmt.Print("\ncanape · a network you haven't set up\n\n")
	}
	row("Wi-Fi", wifiLine(h, link))
	gatewayIP, _, _ := discovery.Router()
	row("Box", boxLine(h, scan, gatewayIP))
	row("Internet", inet.Words())

	byGroup := map[string][]string{}
	count, lgUnpaired := 0, false
	for _, d := range scan.Devices {
		if d.IP == gatewayIP || d.Kind() == "Freebox" {
			continue
		}
		count++
		group := kindGroup[d.Kind()]
		if group == "" {
			group = "Other"
		}
		name := deviceName(d)
		switch {
		case d.Self:
			name = "this computer"
		case d.Kind() == "LG webOS TV" && h.TV.Key != "" && d.IP == h.TV.IP:
			name += " (" + tvState + ")"
		case d.Kind() == "LG webOS TV":
			name += " (not paired yet)"
			lgUnpaired = true
		case d.Kind() == "Samsung TV", d.Kind() == "Sony TV":
			name += " (not supported yet)"
		}
		byGroup[group] = append(byGroup[group], name)
	}
	fmt.Println()
	row("Devices", fmt.Sprintf("%d online", count))
	for _, g := range groups {
		if names := byGroup[g.name]; len(names) > 0 {
			fmt.Printf("  %-9s %-10s %s\n", "", g.name, summarize(names, 4))
		}
	}

	fmt.Println()
	var next [][2]string
	if lgUnpaired || !h.Saved {
		next = append(next, [2]string{"canape setup", "pair the TV and save this Wi-Fi"})
	}
	if h.TV.Key != "" {
		next = append(next, [2]string{"canape tv", "control the TV"})
	}
	if h.Wifi.SSID != "" {
		next = append(next, [2]string{"canape wifi tv", "show the Wi-Fi QR on the TV for guests"})
	} else if h.Saved {
		next = append(next, [2]string{"canape wifi set", "save this Wi-Fi to share it as a QR code"})
	}
	next = append(next, [2]string{"canape serve", "the same, and a remote, on your phone"})
	for i, n := range next {
		label := ""
		if i == 0 {
			label = "Next"
		}
		fmt.Printf("  %-9s %-16s %s\n", label, n[0], n[1])
	}
	return nil
}

func wifiLine(h *home.Here, link *discovery.Link) string {
	name := h.Wifi.SSID
	if name == "" && h.Freebox.AppToken != "" {
		if fbx, err := freebox.New(h.Freebox.AppToken); err == nil {
			name, _ = fbx.SSID()
		}
	}
	if link == nil {
		if name != "" {
			return name + ", this computer isn't on Wi-Fi (cable?)"
		}
		return "this computer isn't on Wi-Fi (cable?)"
	}
	var parts []string
	if name != "" {
		parts = append(parts, name)
	}
	if band := strings.TrimSpace(link.Band + " " + link.Standard); band != "" {
		parts = append(parts, band)
	}
	for _, w := range []string{link.SignalWords(), link.SecurityWords()} {
		if w != "" {
			parts = append(parts, w)
		}
	}
	return strings.Join(parts, ", ")
}

func boxLine(h *home.Here, scan *discovery.Result, gatewayIP string) string {
	model := ""
	if f := scan.Freebox; f != nil {
		model = "Freebox"
		if f.Nick != "" {
			model += " " + f.Nick
		}
	} else {
		for _, d := range scan.Devices {
			if d.IP == gatewayIP {
				model = firstNonEmpty(d.Name, d.Model)
			}
		}
	}
	if model == "" {
		model = "a router"
	}
	line := fmt.Sprintf("%s at %s", model, gatewayIP)
	if h.Freebox.AppToken != "" {
		line += ", paired with canape"
	}
	return line
}

func pairedTVState(h *home.Here) string {
	if h.TV.Key == "" {
		return ""
	}
	t, err := h.ConnectTV()
	if err != nil {
		return "paired, off"
	}
	defer t.Close()
	return "paired, on" + nowShowing(t)
}

// deviceName is the friendliest name a device gives: its announced name, or
// the first part of its network name ("iphone-de-lucas" from "iphone-de-lucas.home").
func deviceName(d *discovery.Device) string {
	host, _, _ := strings.Cut(d.Host, ".")
	return firstNonEmpty(d.Name, d.Alias, host, d.IP)
}

// summarize lists a few names and counts the rest, so a long group stays one line.
func summarize(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:max], ", "), len(names)-max)
}
