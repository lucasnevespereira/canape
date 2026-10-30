package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/home"
)

// setup walks through this network once: name it, pair the TV, pair the
// router, save the Wi-Fi. Each step skips what's already done, so running it
// again only fills the gaps. It never touches other homes.
func setup() error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	in := stdinLines()

	if h.Saved {
		fmt.Printf("Setting up %q.\n", h.Name)
	} else {
		fmt.Println("A network canape doesn't know yet.")
		for {
			name := prompt(in, "Name this home", h.Name)
			if !nameTaken(h, name) {
				h.Name = name
				break
			}
			fmt.Printf("Another home is already called %q.\n", name)
		}
	}

	fmt.Println("\n1. TV")
	if h.TV.Key != "" {
		fmt.Printf("   %s at %s, already paired.\n", home.BrandName(h.TV.Brand), h.TV.IP)
	} else {
		t, err := pairTV(h, "   ")
		switch {
		case errors.Is(err, home.ErrNoTV):
			fmt.Println("   Skipped. If your TV is off, turn it on and run canape setup again.")
		case err != nil:
			fmt.Printf("   %v\n   Skipped. Try again with: canape tv pair\n", err)
		default:
			t.Notify("canape is paired")
			t.Close()
			fmt.Println("   Paired.")
		}
	}

	fmt.Println("\n2. Router")
	switch f := discovery.FindFreebox(); {
	case h.Freebox.AppToken != "":
		fmt.Println("   Freebox, already paired.")
	case f == nil:
		fmt.Println("   Not a Freebox, so nothing to pair. canape scan still finds every device.")
	default:
		fmt.Printf("   Found a Freebox %s. Pairing it, press Enter to skip.\n", f.Nick)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		done := make(chan error, 1)
		go func() { done <- freeboxPair(ctx, h) }()
		select {
		case err = <-done:
		case <-in:
			cancel()
			<-done
			err = errors.New("skipped")
		}
		cancel()
		if err != nil {
			fmt.Printf("   Not paired (%v). Pair it later with: canape freebox pair\n", err)
		}
	}

	fmt.Println("\n3. Wi-Fi to share")
	if h.Wifi.SSID != "" {
		fmt.Printf("   %s, already saved. Change it with: canape wifi set\n", h.Wifi.SSID)
	} else if !askWifi(h, in, "   ") {
		fmt.Println("   Skipped.")
	}

	// Save even if nothing paired, so the name the user chose sticks.
	if err := h.Save(); err != nil {
		return err
	}
	fmt.Printf("\nDone. %q is set up. Try:\n", h.Name)
	fmt.Println("  canape              this network in plain words")
	fmt.Println("  canape wifi tv      the Wi-Fi QR on the TV")
	fmt.Println("  canape serve        web dashboard")
	return nil
}

// otherTVs names the TVs on the network that canape can't drive yet, so a
// failed pairing doesn't look like an empty network.
func otherTVs(indent string) {
	res, err := discovery.Scan()
	if err != nil {
		return
	}
	for _, d := range res.Devices {
		switch k := d.Kind(); k {
		case "Samsung TV", "Sony TV", "Android TV / Google Cast", "Apple TV", "Freebox Player":
			fmt.Printf("%sFound a %s at %s. canape can't control it yet.\n", indent, k, d.IP)
			fmt.Printf("%sWant to help add it? See docs/adding-a-tv.md\n", indent)
		}
	}
}

func nameTaken(h *home.Here, name string) bool {
	for key, other := range h.State.Homes {
		if other.Name == name && key != h.Router {
			return true
		}
	}
	return false
}
