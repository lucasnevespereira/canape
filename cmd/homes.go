package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
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
			answer := prompt(bufio.NewReader(os.Stdin), fmt.Sprintf("Forget %q and its pairings? Type yes", name), "")
			if answer != "yes" {
				fmt.Println("Kept.")
				return nil
			}
			delete(h.State.Homes, key)
			if err := h.State.Save(); err != nil {
				return err
			}
			fmt.Printf("Forgot %q.\n", name)
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
	if !h.Saved {
		fmt.Printf("  → this network (router %s) isn't set up yet. Run: canape setup\n\n", h.Router)
	}
	if len(keys) == 0 {
		fmt.Println("  No homes saved yet.")
		return nil
	}
	for _, k := range keys {
		hm := h.State.Homes[k]
		marker := " "
		if k == h.Router {
			marker = "→"
		}
		var parts []string
		if hm.TV.Key != "" {
			parts = append(parts, fmt.Sprintf("%s TV at %s", strings.ToUpper(hm.TV.Brand), hm.TV.IP))
		}
		if hm.Freebox.AppToken != "" {
			parts = append(parts, "Freebox")
		}
		if hm.Wifi.SSID != "" {
			parts = append(parts, "Wi-Fi "+hm.Wifi.SSID)
		}
		if len(parts) == 0 {
			parts = append(parts, "nothing paired")
		}
		fmt.Printf("%s %-12s %s   %s\n", marker, hm.Name, k, strings.Join(parts, ", "))
	}
	return nil
}
