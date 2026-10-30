package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lucasnevespereira/canape/internal/home"
)

func homesCommand(args []string) error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return listHomes(h)
	}
	switch args[0] {
	case "rename":
		if len(args) < 2 {
			return errors.New("usage: canape homes rename <name>")
		}
		name := strings.Join(args[1:], " ")
		if nameTaken(h, name) {
			return fmt.Errorf("another home is already called %q", name)
		}
		h.Name = name
		if err := h.Save(); err != nil {
			return err
		}
		fmt.Printf("This network is now %q.\n", name)
		return nil
	case "forget":
		if len(args) < 2 {
			return errors.New("usage: canape homes forget <name>")
		}
		name := strings.Join(args[1:], " ")
		for key, other := range h.State.Homes {
			if other.Name != name {
				continue
			}
			delete(h.State.Homes, key)
			if err := h.State.Save(); err != nil {
				return err
			}
			fmt.Printf("Forgot %q and what was paired there.\n", name)
			if key == h.Router {
				fmt.Println("That was this network. Run canape setup to set it up again.")
			}
			return nil
		}
		return fmt.Errorf("no home called %q. Run canape homes to list them", name)
	}
	return fmt.Errorf("unknown command %q. Run canape without arguments for help", args[0])
}

func listHomes(h *home.Here) error {
	keys := make([]string, 0, len(h.State.Homes))
	for k := range h.State.Homes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return h.State.Homes[keys[i]].Name < h.State.Homes[keys[j]].Name })
	if len(keys) == 0 {
		fmt.Println("No homes saved yet. Run canape setup on a network to save it.")
		return nil
	}
	for _, k := range keys {
		hm := h.State.Homes[k]
		here := ""
		if k == h.Router {
			here = "  (you're here)"
		}
		fmt.Printf("%s%s\n", hm.Name, here)
		var parts []string
		if hm.TV.Key != "" {
			parts = append(parts, home.BrandName(hm.TV.Brand)+" TV")
		}
		if hm.Freebox.AppToken != "" {
			parts = append(parts, "Freebox")
		}
		if hm.Wifi.SSID != "" {
			parts = append(parts, "Wi-Fi "+hm.Wifi.SSID)
		}
		for _, c := range hm.Computers {
			parts = append(parts, c.Name)
		}
		if len(parts) == 0 {
			parts = append(parts, "nothing paired yet")
		}
		fmt.Printf("  %s\n", strings.Join(parts, ", "))
	}
	if !h.Saved {
		fmt.Println("\nThis network isn't saved yet. Run canape setup.")
	}
	fmt.Println("\nRename this one: canape homes rename <name>. Forget one: canape homes forget <name>.")
	return nil
}
