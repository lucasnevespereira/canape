package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/router/freebox"
	"github.com/lucasnevespereira/canape/internal/store"
)

// Device kinds, grouped the way a non-expert thinks about them.
var groups = []string{"TVs", "Streaming", "Speakers", "Phones", "Computers", "Printers", "Smart home", "Other"}

var kindGroup = map[string]string{
	"LG webOS TV": "TVs", "Samsung TV": "TVs", "Sony TV": "TVs",
	"Apple TV": "Streaming", "Android TV / Google Cast": "Streaming", "Freebox Player": "Streaming",
	"Speaker": "Speakers", "AirPlay device": "Speakers",
	"iPhone": "Phones", "iPad": "Phones",
	"Mac": "Computers", "this machine": "Computers",
	"Printer": "Printers",
}

// Makers whose devices are almost always one kind of thing, for devices that
// announce nothing. Espressif and Tuya chips run most smart plugs and bulbs.
var makerGroup = map[string]string{
	"Espressif": "Smart home", "Tuya": "Smart home", "Shelly": "Smart home", "Philips Hue": "Smart home",
	"Netatmo": "Smart home", "Sonos": "Speakers",
}

// report is what bare `canape` prints: the network in plain words, then what
// you can do on it. It works on any network, with or without router access.
func report() error {
	h, err := home.Load()
	unknown := errors.Is(err, home.ErrUnknownNetwork)
	if unknown {
		// Nothing saved can apply, but everything else still works.
		h, err = &home.Here{Home: &store.Home{}}, nil
	}
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

	switch {
	case unknown:
		fmt.Print("\ncanape · a network canape can't identify (is a VPN on?)\n\n")
	case h.Saved:
		fmt.Printf("\ncanape · %s\n\n", h.Name)
	default:
		fmt.Print("\ncanape · a network you haven't set up\n\n")
	}
	row("Wi-Fi", wifiLine(h, link))
	if link != nil && link.Details() != "" {
		row("", link.Details())
	}
	gatewayIP, _, _ := discovery.Router()
	row("Box", boxLine(h, scan, gatewayIP))
	row("Internet", inet.Words())

	byGroup := map[string][]string{}
	count, lgUnpaired, tvSeen := 0, false, false
	for _, d := range scan.Devices {
		if d.IP == gatewayIP || d.Kind() == "Freebox" {
			continue
		}
		count++
		group := kindGroup[d.Kind()]
		if group == "" {
			group = makerGroup[d.Vendor]
		}
		if group == "" {
			group = "Other"
		}
		name := deviceName(d)
		switch {
		case d.Self:
			name = "this computer"
		case d.Kind() == "LG webOS TV" && h.TV.Key != "" && d.IP == h.TV.IP:
			name = home.BrandName(h.TV.Brand) + " TV (" + tvState + ")"
			tvSeen = true
		case d.Kind() == "LG webOS TV":
			name += " (not paired yet)"
			lgUnpaired = true
		case d.Kind() == "Samsung TV", d.Kind() == "Sony TV":
			name += " (not supported yet)"
		}
		byGroup[group] = append(byGroup[group], name)
	}
	if h.TV.Key != "" && !tvSeen {
		// A TV that's fully off doesn't answer the scan, but it's still there.
		byGroup["TVs"] = append(byGroup["TVs"], home.BrandName(h.TV.Brand)+" TV ("+tvState+")")
	}
	fmt.Println()
	row("Devices", fmt.Sprintf("%d online", count))
	for _, g := range groups {
		if names := byGroup[g]; len(names) > 0 {
			fmt.Printf("  %-9s %-10s %s\n", "", g, summarize(names, 4))
		}
	}

	fmt.Println()
	var next [][2]string
	switch {
	case unknown:
	case lgUnpaired:
		next = append(next, [2]string{"canape setup", "pair the LG TV"})
	case !h.Saved:
		next = append(next, [2]string{"canape setup", "save this network and its Wi-Fi"})
	}
	if h.TV.Key != "" {
		next = append(next, [2]string{"canape tv", "control the TV"})
	}
	if h.Wifi.SSID != "" {
		next = append(next, [2]string{"canape wifi tv", "show the Wi-Fi QR on the TV for guests"})
	} else if h.Saved {
		next = append(next, [2]string{"canape wifi set", "save this Wi-Fi to share it as a QR code"})
	}
	if h.TV.Key != "" || h.Wifi.SSID != "" {
		next = append(next, [2]string{"canape serve", "do it from your phone"})
	}
	next = append(next, [2]string{"canape scan", "every device in detail"})
	next = append(next, [2]string{"canape help", "everything canape can do"})
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
// the first part of its network name ("iphone-de-alex" from "iphone-de-alex.home").
// The maker fills in when the network name says little.
func deviceName(d *discovery.Device) string {
	alias := d.Alias
	if uuidRe.MatchString(alias) || alias == "SpotifyConnect" {
		alias = "" // iPhones announce a random ID instead of their name, speakers the service
	}
	if name := firstNonEmpty(d.Name, alias); name != "" {
		return name
	}
	host, _, _ := strings.Cut(d.Host, ".")
	if junkHost.MatchString(strings.ToLower(host)) {
		host = ""
	}
	brand, _, _ := strings.Cut(strings.ToLower(d.Vendor), " ")
	switch {
	case host != "" && d.Vendor != "" && !strings.ContainsAny(host, "-_") && !strings.Contains(strings.ToLower(host), brand):
		return d.Vendor + " " + host // "c210" says little, "TP-Link c210" says a camera
	case host != "":
		return host
	case d.Vendor != "":
		return d.Vendor + " device"
	}
	return d.IP
}

var uuidRe = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// Network names that say nothing: chip defaults ("lwip0", "espressif",
// "esp_1a2b3c"), bare "android", and the long random ones macOS hands out.
var junkHost = regexp.MustCompile(`^(lwip\d*|espressif|esp[-_]?[0-9a-f]*|android(-[0-9a-f]+)?|localhost|unknown)$|[0-9a-z]{20,}`)

// summarize lists a few names and counts the rest, so a long group stays one line.
func summarize(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:max], ", "), len(names)-max)
}
