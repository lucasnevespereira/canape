package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/lucasnevespereira/canape/internal/home"
)

func wakeCommand(args []string) error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	switch {
	case len(args) == 0:
		return listComputers(h)
	case args[0] == "add":
		return addComputer(h)
	case args[0] == "forget":
		if len(args) < 2 {
			return errors.New("usage: canape wake forget <name>")
		}
		if err := h.ForgetComputer(strings.Join(args[1:], " ")); err != nil {
			return err
		}
		fmt.Println("Forgotten.")
		return nil
	}
	c, err := h.WakeComputer(strings.Join(args, " "))
	if err != nil {
		return err
	}
	fmt.Printf("Wake-on-LAN sent to %s. It can take a minute to come up.\n", c.Name)
	return nil
}

func listComputers(h *home.Here) error {
	if len(h.Computers) == 0 {
		fmt.Println("No computer saved for waking yet. With the computer on, run: canape wake add")
		return nil
	}
	online := h.Online()
	for _, c := range h.Computers {
		status := ""
		if online != nil {
			status = map[bool]string{true: "awake", false: "asleep or off"}[online[c.MAC]]
		}
		fmt.Printf("  %-24s %-17s %s\n", c.Name, c.MAC, status)
	}
	fmt.Println("\nWake one with: canape wake <name>")
	return nil
}

// addComputer lists what's online now and saves the one the user picks.
func addComputer(h *home.Here) error {
	fmt.Println("Looking at what's online (the computer you want to add must be on)...")
	list, err := h.Candidates()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return errors.New("nothing new online. Turn the computer on and run this again")
	}
	for i, c := range list {
		vendor := ""
		if c.Vendor != "" {
			vendor = ", " + c.Vendor
		}
		fmt.Printf("  %2d. %s (%s%s)\n", i+1, c.Name, c.IP, vendor)
	}
	in := bufio.NewReader(os.Stdin)
	n, err := strconv.Atoi(strings.TrimSpace(prompt(in, "\nWhich one? Type its number", "")))
	if err != nil || n < 1 || n > len(list) {
		return errors.New("no computer picked")
	}
	c := list[n-1]
	name := prompt(in, "Name it", c.Name)
	if err := h.AddComputer(name, c.MAC, c.IP); err != nil {
		return err
	}
	fmt.Printf("Saved. Wake it with: canape wake %s\n", name)
	fmt.Println(`Its own settings must allow it: "Wake on LAN" in a PC's BIOS, "Wake for network access" on a Mac.`)
	return nil
}
