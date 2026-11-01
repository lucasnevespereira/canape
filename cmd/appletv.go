package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/lucasnevespereira/canape/internal/appletv"
	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/store"
)

const appleTVUsage = `  canape appletv play | pause       play or pause
  canape appletv forward | rewind   skip 10 seconds ahead or back (or: forward 30)
  canape appletv up | down | left | right | ok | back | home
  canape appletv volume up|down     the volume, through the TV
  canape appletv sleep | wake`

// Words people type, mapped to the remote button they mean.
var atvButtons = map[string]string{
	"up": "up", "down": "down", "left": "left", "right": "right", "ok": "select", "select": "select",
	"back": "menu", "menu": "menu", "home": "home", "play": "playpause", "pause": "playpause",
	"volume-up": "volume-up", "volume-down": "volume-down", "sleep": "sleep", "wake": "wake",
}

func appleTVCommand(args []string) error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		args = []string{"status"}
	}
	switch args[0] {
	case "pair":
		err := pairAppleTV(h, bufio.NewReader(os.Stdin), "")
		if errors.Is(err, errSkipped) {
			fmt.Println("Not paired.")
			return nil
		}
		return err
	case "status":
		if h.AppleTV == nil {
			return errors.New("no Apple TV paired on this network. Run: canape appletv pair")
		}
		c, err := h.ConnectAppleTV()
		if err != nil {
			return err
		}
		c.Close()
		fmt.Printf("%s (Apple TV) at %s: paired and answering\n\n", h.AppleTV.Name, h.AppleTV.IP)
		fmt.Println(appleTVUsage)
		return nil
	}

	c, err := h.ConnectAppleTV()
	if err != nil {
		return err
	}
	defer c.Close()
	switch cmd := args[0]; cmd {
	case "forward", "rewind":
		secs := 10.0
		if len(args) > 1 {
			if secs, err = strconv.ParseFloat(args[1], 64); err != nil {
				return fmt.Errorf("usage: canape appletv %s [seconds]", cmd)
			}
		}
		if cmd == "rewind" {
			secs = -secs
		}
		return c.Skip(secs)
	case "volume":
		if len(args) < 2 || (args[1] != "up" && args[1] != "down") {
			return errors.New("usage: canape appletv volume up|down")
		}
		return c.Press("volume-" + args[1])
	default:
		b, ok := atvButtons[cmd]
		if !ok {
			return fmt.Errorf("unknown command %q. Try:\n%s", cmd, appleTVUsage)
		}
		return c.Press(b)
	}
}

var (
	errNoAppleTV = errors.New("no Apple TV answered. Turn it on, on this Wi-Fi, and try again")
	errSkipped   = errors.New("skipped")
)

// pairAppleTV finds the Apple TV, has it show a code, and saves the pairing.
func pairAppleTV(h *home.Here, in lineReader, indent string) error {
	fmt.Println(indent + "Looking for an Apple TV (a few seconds)...")
	found, err := home.FindAppleTVs()
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return errNoAppleTV
	}
	atv := found[0]
	if len(found) > 1 {
		for i, a := range found {
			fmt.Printf("%s  %d. %s\n", indent, i+1, a.Name)
		}
		n, _ := strconv.Atoi(prompt(in, indent+"Which one", "1"))
		if n < 1 || n > len(found) {
			return errors.New("no Apple TV with that number")
		}
		atv = found[n-1]
	}
	p, err := appletv.StartPairing(fmt.Sprintf("%s:%d", atv.IP, atv.Port))
	if err != nil {
		return err
	}
	defer p.Close()
	code := prompt(in, fmt.Sprintf("%sType the code on %s's screen (Enter skips)", indent, atv.Name), "")
	if code == "" {
		return errSkipped
	}
	cr, err := p.Finish(code)
	if err != nil {
		return err
	}
	h.AppleTV = &store.AppleTV{Name: atv.Name, IP: atv.IP, Port: atv.Port, Pairing: *cr}
	if err := h.Save(); err != nil {
		return err
	}
	fmt.Printf("%sPaired with %s. Try: canape appletv pause\n", indent, atv.Name)
	return nil
}
