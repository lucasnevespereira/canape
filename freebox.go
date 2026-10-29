package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/lucasnevespereira/canape/freebox"
)

func freeboxCommand(cmd string, args []string) error {
	s, err := loadState()
	if err != nil {
		return err
	}
	if cmd == "pair" {
		return freeboxPair(s)
	}
	if s.Freebox.AppToken == "" {
		return errors.New("not paired with the Freebox yet. Run: canape freebox pair")
	}
	fbx, err := freebox.New(s.Freebox.AppToken)
	if err != nil {
		return err
	}
	switch cmd {
	case "devices":
		return freeboxDevices(fbx, len(args) > 0 && args[0] == "all")
	default:
		return fmt.Errorf("unknown command %q. Run canape without arguments for help", cmd)
	}
}

func freeboxPair(s *state) error {
	device, _ := os.Hostname()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	token, err := freebox.Authorize(ctx, "canape", "0.1", device, func() {
		fmt.Println("Look at the Freebox screen: use its arrows to pick \"Oui\" and confirm.")
	})
	if err != nil {
		return err
	}
	s.Freebox.AppToken = token
	if err := saveState(s); err != nil {
		return err
	}
	fmt.Println("Paired. Saved to", statePath())

	fbx, err := freebox.New(token)
	if err != nil {
		return err
	}
	if err := fbx.Login(); err != nil {
		return err
	}
	if !fbx.Perms["settings"] {
		fmt.Print(`
One more step for guest Wi-Fi: canape needs permission to change settings.
  1. Open http://mafreebox.freebox.fr and log in
  2. Paramètres de la Freebox > Gestion des accès > Applications
  3. Edit "canape" and tick "Modification des réglages de la Freebox"
`)
	}
	fmt.Println("Try: canape freebox devices")
	return nil
}

func freeboxDevices(fbx *freebox.Client, all bool) error {
	hosts, err := fbx.Hosts()
	if err != nil {
		return err
	}
	offline := 0
	for _, h := range hosts {
		if !h.Reachable {
			offline++
			if !all {
				continue
			}
		}
		dot, seen := "●", "online"
		if !h.Reachable {
			dot, seen = "○", ago(time.Unix(h.LastActivity, 0))
		}
		fmt.Printf("  %s %-28.28s %-15s %-17s %-18.18s %s\n", dot, h.Name, h.IPv4(), h.MAC(), h.Vendor, seen)
	}
	if !all && offline > 0 {
		fmt.Printf("\n  %d more seen before. Run: canape freebox devices all\n", offline)
	}
	return nil
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}
