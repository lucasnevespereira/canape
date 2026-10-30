package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/router/freebox"
	"github.com/lucasnevespereira/canape/internal/store"
)

func freeboxCommand(cmd string, args []string) error {
	s, err := home.Load()
	if err != nil {
		return err
	}
	switch cmd {
	case "pair":
		return freeboxPair(s)
	case "devices":
		return devices(s, len(args) > 0 && args[0] == "all")
	default:
		return fmt.Errorf("unknown command %q. Run canape without arguments for help", cmd)
	}
}

func freeboxPair(s *home.Here) error {
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
	if err := s.Save(); err != nil {
		return err
	}
	fmt.Printf("Paired. Saved in home %q (%s)\n", s.Name, store.Path())
	fmt.Println("Try: canape freebox devices")
	return nil
}

func devices(s *home.Here, all bool) error {
	hosts, err := s.Devices(true)
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
