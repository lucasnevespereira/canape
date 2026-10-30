package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/store"
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
	}

	t, err := s.ConnectTV()
	if err != nil {
		return err
	}
	defer t.Close()

	switch cmd {
	case "toast":
		if len(args) == 0 {
			return errors.New(`usage: canape tv toast "message"`)
		}
		err = t.Notify(strings.Join(args, " "))
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
	case "button":
		if len(args) == 0 {
			return errors.New("usage: canape tv button <home|back|up|down|left|right|enter>")
		}
		err = t.Key(strings.ToUpper(args[0]))
	case "type":
		text := strings.Join(args, " ")
		if text == "" {
			// Prompting keeps passwords out of the shell history.
			text = prompt(bufio.NewReader(os.Stdin), "Text to type on the TV", "")
		}
		err = t.Type(text)
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
		return fmt.Errorf("unknown command %q. Run canape without arguments for help", cmd)
	}
	return err
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
	fmt.Println("Launched", c.Label)
	return nil
}

func pair(s *home.Here) error {
	fmt.Print("Looking for a TV... ")
	t, err := s.PairTV(func(brand, ip string) {
		fmt.Printf("found %s at %s\n", brand, ip)
	}, func() {
		fmt.Println("Accept the prompt on your TV (you have 60 seconds)...")
	})
	if err != nil {
		fmt.Println()
		return err
	}
	defer t.Close()
	fmt.Printf("Paired. Saved in home %q (%s)\n", s.Name, store.Path())
	if s.TV.MAC == "" {
		fmt.Println(`Couldn't read the TV's MAC address, so "canape tv on" won't work. Add it to the file by hand.`)
	}
	t.Notify("canape is paired")
	fmt.Println(`Try: canape tv toast "hello from the couch"`)
	return nil
}

// installApp builds the Wi-Fi QR app and installs it through LG's Developer
// Mode. The QR is baked in, so it works with canape stopped; run it again
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
	if app.Icon, err = wifi.PNG(text, 80); err != nil {
		return err
	}
	if app.LargeIcon, err = wifi.PNG(text, 130); err != nil {
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
