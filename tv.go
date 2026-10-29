package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/lgtv"
)

func tvCommand(cmd string, args []string) error {
	s, err := loadState()
	if err != nil {
		return err
	}
	switch cmd {
	case "pair":
		return pair(s)
	case "on":
		if s.TV.MAC == "" {
			return errors.New("no MAC address saved for the TV. Run: canape tv pair (with the TV on)")
		}
		if err := lgtv.Wake(s.TV.MAC, s.TV.IP); err != nil {
			return err
		}
		fmt.Println(`Wake-on-LAN sent. If the TV stays off, enable "Turn on via Wi-Fi" in its settings.`)
		return nil
	}

	tv, err := connect(s)
	if err != nil {
		return err
	}
	defer tv.Close()

	switch cmd {
	case "toast":
		if len(args) == 0 {
			return errors.New(`usage: canape tv toast "message"`)
		}
		_, err = tv.Call("system.notifications/createToast", map[string]string{"message": strings.Join(args, " ")})
	case "off":
		_, err = tv.Call("system/turnOff", nil)
		if err == nil {
			fmt.Println("TV off. Turn it back on with: canape tv on")
		}
	case "volume":
		if len(args) == 0 {
			return printCall(tv, "audio/getVolume", nil)
		}
		n, convErr := strconv.Atoi(args[0])
		if convErr != nil || n < 0 || n > 100 {
			return errors.New("volume must be a number from 0 to 100")
		}
		_, err = tv.Call("audio/setVolume", map[string]int{"volume": n})
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
		return printCall(tv, args[0], payload)
	default:
		return fmt.Errorf("unknown command %q. Run canape without arguments for help", cmd)
	}
	return err
}

func pair(s *state) error {
	fmt.Print("Looking for an LG TV... ")
	ip, err := lgtv.Discover(3 * time.Second)
	if err != nil {
		fmt.Println()
		return err
	}
	fmt.Println(ip)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tv, err := lgtv.Dial(ctx, ip)
	if err != nil {
		return err
	}
	defer tv.Close()
	key, err := tv.Register(s.TV.Key, func() {
		fmt.Println("Accept the prompt on your TV (you have 60 seconds)...")
	})
	if err != nil {
		return err
	}

	s.TV.IP, s.TV.Key, s.TV.MAC = ip, key, lgtv.MAC(ip)
	if err := saveState(s); err != nil {
		return err
	}
	fmt.Println("Paired. Saved to", statePath())
	if s.TV.MAC == "" {
		fmt.Println(`Couldn't read the TV's MAC address, so "canape tv on" won't work. Add it to the file by hand.`)
	}
	tv.Call("system.notifications/createToast", map[string]string{"message": "canape is paired"})
	fmt.Println(`Try: canape tv toast "hello from the couch"`)
	return nil
}

// connect dials the saved TV and authenticates with the saved key.
func connect(s *state) (*lgtv.TV, error) {
	if s.TV.Key == "" {
		return nil, errors.New("not paired yet. Run: canape tv pair")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tv, err := lgtv.Dial(ctx, s.TV.IP)
	if err != nil {
		return nil, fmt.Errorf("%w\nIs the TV on? If its IP changed, run: canape tv pair", err)
	}
	key, err := tv.Register(s.TV.Key, func() {
		fmt.Println("The TV forgot canape. Accept the prompt on screen...")
	})
	if err != nil {
		tv.Close()
		return nil, err
	}
	if key != s.TV.Key {
		s.TV.Key = key
		if err := saveState(s); err != nil {
			tv.Close()
			return nil, err
		}
	}
	return tv, nil
}

func printCall(tv *lgtv.TV, uri string, payload any) error {
	out, err := tv.Call(uri, payload)
	if len(out) > 0 {
		var pretty any
		if json.Unmarshal(out, &pretty) == nil {
			b, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(b))
		}
	}
	return err
}
