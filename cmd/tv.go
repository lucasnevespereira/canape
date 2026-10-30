package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/qrcode"
	"github.com/lucasnevespereira/canape/internal/tv"
	"github.com/lucasnevespereira/canape/internal/tv/lg"
	"github.com/lucasnevespereira/canape/internal/wifi"
)

func tvCommand(cmd string, args []string) error {
	s, err := home.Load()
	if err != nil {
		return err
	}
	switch cmd {
	case "pair":
		return pair(s)
	case "install-app":
		return installApp(s)
	case "on":
		if err := s.WakeTV(); err != nil {
			return err
		}
		fmt.Println(`Wake-on-LAN sent. If the TV stays off, enable "Turn on via Wi-Fi" in its settings.`)
		return nil
	case "status":
		if s.TV.Key == "" {
			return errors.New("no TV paired on this network. Run: canape setup")
		}
		t, err := s.ConnectTV()
		if err != nil {
			fmt.Printf("%s at %s: off or unreachable. Turn it on with: canape tv on\n", home.BrandName(s.TV.Brand), s.TV.IP)
			return nil
		}
		defer t.Close()
		v, err := t.Volume()
		if err != nil {
			return err
		}
		fmt.Printf("%s at %s: on%s, volume %d%s\n", home.BrandName(s.TV.Brand), s.TV.IP, nowShowing(t), v.Level, map[bool]string{true: " (muted)"}[v.Muted])
		return nil
	}

	t, err := s.ConnectTV()
	if err != nil {
		return err
	}
	defer t.Close()

	switch cmd {
	case "off":
		err = t.TurnOff()
		if err == nil {
			fmt.Println("TV off. Turn it back on with: canape tv on")
		}
	case "volume":
		if len(args) == 0 {
			v, err := t.Volume()
			if err == nil {
				fmt.Printf("Volume %d%s\n", v.Level, map[bool]string{true: " (muted)"}[v.Muted])
			}
			return err
		}
		n, convErr := strconv.Atoi(args[0])
		if convErr != nil || n < 0 || n > 100 {
			return errors.New("volume must be a number from 0 to 100")
		}
		err = t.SetVolume(n)
	case "mute":
		var mute bool
		switch {
		case len(args) == 0: // toggle
			v, err := t.Volume()
			if err != nil {
				return err
			}
			mute = !v.Muted
		case args[0] == "on":
			mute = true
		case args[0] == "off":
		default:
			return errors.New("usage: canape tv mute [on|off]")
		}
		if err = t.SetMute(mute); err == nil {
			fmt.Println(map[bool]string{true: "Muted.", false: "Unmuted."}[mute])
		}
	case "key":
		if len(args) == 0 {
			return errors.New("usage: canape tv key <home|back|up|down|left|right|ok>")
		}
		key := strings.ToUpper(args[0])
		if key == "OK" {
			key = tv.KeyOK
		}
		if !slices.Contains(tv.Keys, key) {
			return fmt.Errorf("unknown key %q. Keys: home, back, up, down, left, right, ok", args[0])
		}
		err = t.Key(key)
	case "type":
		text := strings.Join(args, " ")
		if text == "" {
			// Prompting keeps passwords out of the shell history.
			text = prompt(bufio.NewReader(os.Stdin), "Text to type on the TV", "")
		}
		err = t.Type(text)
	case "notify":
		if len(args) == 0 {
			return errors.New(`usage: canape tv notify "message"`)
		}
		err = t.Notify(strings.Join(args, " "))
	case "input":
		err = tvInput(t, args)
	case "app":
		err = tvApp(t, args)
	case "open":
		if len(args) == 0 {
			return errors.New("usage: canape tv open <url>")
		}
		err = t.Open(args[0])
	case "call":
		if len(args) == 0 {
			return errors.New("usage: canape tv call <uri> [json payload]")
		}
		var payload any
		if len(args) > 1 {
			if err := json.Unmarshal([]byte(args[1]), &payload); err != nil {
				return fmt.Errorf("payload is not valid JSON: %w", err)
			}
		}
		lt, ok := t.(*lg.TV)
		if !ok {
			return errors.New("tv call sends raw LG requests, so it only works on LG TVs")
		}
		return printCall(lt, args[0], payload)
	default:
		return fmt.Errorf("unknown command %q. Run: canape help", "tv "+cmd)
	}
	if errors.Is(err, tv.ErrUnsupported) {
		return fmt.Errorf("%s TVs can't do %q", home.BrandName(s.TV.Brand), cmd)
	}
	return err
}

// nowShowing names what's on screen, e.g. ", showing Apple", or "" if unknown.
func nowShowing(t tv.TV) string {
	id, err := t.Current()
	if err != nil || id == "" {
		return ""
	}
	if inputs, err := t.Inputs(); err == nil {
		for _, in := range inputs {
			if in.ID == id {
				return ", showing " + in.Label
			}
		}
	}
	if apps, err := t.Apps(); err == nil {
		for _, a := range apps {
			if a.ID == id {
				return ", showing " + a.Title
			}
		}
	}
	return ", showing " + id
}

func tvInput(t tv.TV, args []string) error {
	inputs, err := t.Inputs()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		current, _ := t.Current()
		for _, in := range inputs {
			var notes []string
			if in.Connected {
				notes = append(notes, "connected")
			}
			if in.ID == current {
				notes = append(notes, "on screen")
			}
			fmt.Printf("  %-8s %-12s %s\n", in.ID, in.Label, strings.Join(notes, ", "))
		}
		return nil
	}
	var choices []tv.Choice
	for _, in := range inputs {
		choices = append(choices, tv.Choice{ID: in.ID, Label: in.Label})
	}
	c, err := tv.Match(strings.Join(args, " "), choices)
	if err != nil {
		return fmt.Errorf("%w. Run canape tv input to list them", err)
	}
	if err := t.SwitchInput(c.ID); err != nil {
		return err
	}
	fmt.Println("Switched to", c.Label)
	return nil
}

func tvApp(t tv.TV, args []string) error {
	apps, err := t.Apps()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		for _, a := range apps {
			fmt.Printf("  %-28s %s\n", a.Title, a.ID)
		}
		return nil
	}
	var choices []tv.Choice
	for _, a := range apps {
		choices = append(choices, tv.Choice{ID: a.ID, Label: a.Title})
	}
	c, err := tv.Match(strings.Join(args, " "), choices)
	if err != nil {
		return fmt.Errorf("%w. Run canape tv app to list them", err)
	}
	if err := t.Launch(c.ID); err != nil {
		return err
	}
	fmt.Println("Opened", c.Label)
	return nil
}

// pairTV finds a TV and pairs it, saying what happens along the way. When
// none answers, it names the TVs canape found but can't control yet.
func pairTV(h *home.Here, indent string) (tv.TV, error) {
	fmt.Print(indent + "Looking for a TV... ")
	t, err := h.PairTV(func(brand, ip string) {
		fmt.Printf("found the %s TV at %s\n", brand, ip)
	}, func() {
		fmt.Println(indent + "Accept canape on the TV screen (you have 60 seconds)...")
	})
	if errors.Is(err, home.ErrNoTV) {
		fmt.Println("none answered.")
		otherTVs(indent)
	}
	return t, err
}

func pair(s *home.Here) error {
	t, err := pairTV(s, "")
	if err != nil {
		return err
	}
	defer t.Close()
	fmt.Printf("Paired. Saved in home %q.\n", s.Name)
	if s.TV.MAC == "" {
		fmt.Println(`Couldn't read the TV's network address, so "canape tv on" won't work yet. Run canape tv pair again later.`)
	}
	t.Notify("canape is paired")
	fmt.Println(`Try: canape tv notify "hello from the couch"`)
	return nil
}

// installApp builds the Wi-Fi QR app and installs it through LG's Developer
// Mode. The QR is baked in, so it works with canape stopped. Run it again
// after changing the Wi-Fi password.
func installApp(s *home.Here) error {
	if s.Wifi.SSID == "" {
		return errors.New("no Wi-Fi saved yet. Run: canape wifi set")
	}
	if s.TV.Brand != "" && s.TV.Brand != "lg" {
		return errors.New("the TV app uses LG's Developer Mode, so it only works on LG TVs. Use canape wifi tv instead")
	}
	ip := s.TV.IP
	if ip == "" {
		var err error
		if ip, err = lg.Discover(3 * time.Second); err != nil {
			return err
		}
	}
	text := wifi.QRText(s.Wifi.SSID, s.Wifi.Password)
	app := lg.WebApp{ID: "com.canape.wifi", Title: "Wi-Fi"}
	var page strings.Builder
	if err := wifi.Page(&page, s.Wifi.SSID, s.Wifi.Password); err != nil {
		return err
	}
	app.HTML = []byte(page.String())
	var err error
	if app.Icon, err = qrcode.PNG(text, 80); err != nil {
		return err
	}
	if app.LargeIcon, err = qrcode.PNG(text, 130); err != nil {
		return err
	}
	if err := lg.InstallApp(ip, app); err != nil {
		return err
	}
	fmt.Println("\nInstalled. The Wi-Fi app is in the TV's app bar. Run this again after changing the Wi-Fi password.")
	return nil
}

func printCall(t *lg.TV, uri string, payload any) error {
	out, err := t.Call(uri, payload)
	if len(out) > 0 {
		var pretty any
		if json.Unmarshal(out, &pretty) == nil {
			b, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(b))
		}
	}
	return err
}
