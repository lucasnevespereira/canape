package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/router/freebox"
)

func freeboxPairCommand() error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	if err := freeboxPair(h); err != nil {
		return err
	}
	fmt.Println("Try: canape devices")
	return nil
}

// freeboxPair asks the box for an app token. The owner confirms on its screen.
func freeboxPair(h *home.Here) error {
	device, _ := os.Hostname()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	token, err := freebox.Authorize(ctx, "canape", "0.1", device, func() {
		fmt.Println("   Look at the Freebox screen: use its arrows to pick \"Oui\" and confirm.")
	})
	if err != nil {
		return err
	}
	h.Freebox.AppToken = token
	if err := h.Save(); err != nil {
		return err
	}
	fmt.Println("   Freebox paired.")
	return nil
}

func devices(all bool) error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	hosts, err := h.Devices(true)
	if err != nil {
		return err
	}
	offline := 0
	for _, host := range hosts {
		if !host.Reachable {
			offline++
			if !all {
				continue
			}
		}
		dot, seen := "●", "online"
		if !host.Reachable {
			dot, seen = "○", ago(time.Unix(host.LastActivity, 0))
		}
		fmt.Printf("  %s %-28.28s %-15s %-17s %-18.18s %s\n", dot, host.Name, host.IPv4(), host.MAC(), host.Vendor, seen)
	}
	if !all && offline > 0 {
		fmt.Printf("\n  %d more seen before. Run: canape devices all\n", offline)
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
