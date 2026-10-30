// Package cmd is the canape command: argument parsing and terminal output.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/mcp"
	"github.com/lucasnevespereira/canape/internal/server"
	"github.com/lucasnevespereira/canape/internal/store"
	"github.com/lucasnevespereira/canape/internal/version"
)

const usage = `canape controls the TV and shares the Wi-Fi, from the couch or from Claude.

Get started
  canape setup                 pair this network's TV and router, save the Wi-Fi (safe to rerun)
  canape                       what's set up here, and what to do next

TV
  canape tv on | off | status
  canape tv volume [0-100]     show or set the volume
  canape tv mute [on|off]      toggle mute, or set it
  canape tv input [name]       list inputs, or switch to one (e.g. hdmi1, apple)
  canape tv app [name]         list apps, or open one (e.g. youtube)
  canape tv open <url>         open a web page on the TV
  canape tv key <key>          press home, back, up, down, left, right or ok
  canape tv type [text]        type into the selected text field (asks if no text)
  canape tv notify <message>   show a message on the TV

Wi-Fi
  canape wifi                  the Wi-Fi QR code, in the terminal
  canape wifi tv               the Wi-Fi QR code, on the TV
  canape wifi set              change the saved Wi-Fi name or password

Network
  canape scan                  look around: every device on this network (saves nothing)
  canape devices [all]         who's online, from the router's own list (Freebox)
  canape homes                 saved homes, one per network · rename <name> · forget <name>

Run
  canape serve [addr]          phone dashboard (default :8080)
  canape mcp                   MCP server for Claude: claude mcp add canape -- canape mcp
  canape version               which canape this is

Advanced
  canape tv pair               pair a TV again (setup does it the first time)
  canape freebox pair          pair the Freebox again
  canape tv install-app        install a Wi-Fi QR app on an LG TV (needs Developer Mode)
  canape tv call <uri> [json]  raw LG request, e.g. canape tv call audio/getVolume

State: %s
`

// Run runs one command and returns the process exit code.
func Run(args []string) int {
	cmd, rest := "", []string(nil)
	if len(args) > 0 {
		cmd, rest = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "":
		err = status()
	case "help", "-h", "--help":
		fmt.Printf(usage, store.Path())
	case "version", "-v", "--version":
		fmt.Println(version.String())
	case "setup":
		err = setup()
	case "tv":
		if len(rest) == 0 {
			rest = []string{"status"}
		}
		err = tvCommand(rest[0], rest[1:])
	case "wifi":
		err = wifiCommand(rest)
	case "scan":
		err = scan()
	case "devices":
		err = devices(len(rest) > 0 && rest[0] == "all")
	case "homes":
		err = homesCommand(rest)
	case "serve":
		addr := ":8080"
		if len(rest) > 0 {
			addr = rest[0]
		}
		err = server.Run(addr)
	case "mcp":
		err = mcp.Run()
	case "freebox":
		if len(rest) > 0 && rest[0] == "pair" {
			err = freeboxPairCommand()
		} else {
			err = fmt.Errorf("unknown command. Did you mean: canape freebox pair, or canape devices?")
		}
	default:
		err = fmt.Errorf("unknown command %q. Run: canape help", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "canape:", err)
		return 1
	}
	return 0
}

// status is what bare `canape` prints: this network's home and what's next.
func status() error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	if !h.Saved {
		fmt.Printf("canape · new network (router %s)\n\nNothing set up here yet. Run: canape setup\n", h.Router)
		return nil
	}
	fmt.Printf("canape · home %q (router %s)\n\n", h.Name, h.Router)

	tvLine := "not paired"
	if h.TV.Key != "" {
		tvLine = fmt.Sprintf("%s at %s", home.BrandName(h.TV.Brand), h.TV.IP)
		if t, err := h.ConnectTV(); err != nil {
			tvLine += " · off or unreachable (canape tv on)"
		} else {
			tvLine += " · on" + nowShowing(t)
			if v, err := t.Volume(); err == nil {
				tvLine += fmt.Sprintf(", volume %d", v.Level)
			}
			t.Close()
		}
	}
	row("TV", tvLine)
	if h.Freebox.AppToken != "" {
		row("Router", "Freebox, paired")
	} else {
		row("Router", "not paired")
	}
	if h.Wifi.SSID != "" {
		row("Wi-Fi", h.Wifi.SSID+", QR ready (canape wifi tv)")
	} else {
		row("Wi-Fi", "not saved")
	}

	if h.TV.Key == "" || h.Wifi.SSID == "" {
		fmt.Println("\nFinish with: canape setup")
	} else {
		fmt.Println("\nNext: canape serve for the phone dashboard, or canape help")
	}
	return nil
}

func row(label, value string) { fmt.Printf("  %-8s %s\n", label, value) }

func prompt(in *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := in.ReadString('\n')
	// Only the newline: spaces can be part of a Wi-Fi password.
	if line = strings.TrimRight(line, "\r\n"); line == "" {
		return def
	}
	return line
}

// yes asks a yes/no question that defaults to yes.
func yes(in *bufio.Reader, question string) bool {
	a := strings.ToLower(strings.TrimSpace(prompt(in, question+" [Y/n]", "")))
	return a == "" || a == "y" || a == "yes" || a == "o" || a == "oui"
}
