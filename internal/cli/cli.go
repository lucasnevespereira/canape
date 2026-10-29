// Package cli is the canape command: argument parsing and terminal output.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lucasnevespereira/canape/internal/mcp"
	"github.com/lucasnevespereira/canape/internal/server"
	"github.com/lucasnevespereira/canape/internal/store"
)

const usage = `canape controls your TV from the couch.

  canape serve [addr]          run the dashboard (default :8080)
  canape mcp                   MCP server over stdio, for Claude and other MCP clients
  canape scan                  list the devices on this network

  canape tv pair               find the TV and pair (accept the prompt on screen)
  canape tv toast <message>    show a notification on the TV
  canape tv off                turn the TV off
  canape tv on                 turn the TV on (Wake-on-LAN)
  canape tv volume [0-100]     show or set the volume
  canape tv input [name]       list inputs, or switch to one (e.g. hdmi1, apple)
  canape tv app [name]         list apps, or launch one (e.g. youtube)
  canape tv open <url>         open a web page on the TV
  canape tv button <key>       press a remote key: home, back, up, down, left, right, enter
  canape tv type [text]        type into the focused text field (prompts if no text)
  canape tv install-app        install a Wi-Fi QR app on the TV (needs Developer Mode)
  canape tv call <uri> [json]  send any SSAP request and print the reply,
                               e.g. canape tv call audio/getVolume

  canape wifi set              save the Wi-Fi name and password (name comes from the Freebox)
  canape wifi                  show the Wi-Fi QR code in the terminal
  canape wifi tv               show the Wi-Fi QR on the TV (no server needed)

  canape freebox pair          authorize canape on the Freebox (confirm on its screen)
  canape freebox devices [all] devices online now, or every device seen

State is kept in %s (set CANAPE_DATA to move it).
`

// Run runs one command and returns the process exit code.
func Run(args []string) int {
	var err error
	switch {
	case len(args) >= 1 && args[0] == "serve":
		addr := ":8080"
		if len(args) > 1 {
			addr = args[1]
		}
		err = server.Run(addr)
	case len(args) >= 1 && args[0] == "mcp":
		err = mcp.Run()
	case len(args) >= 1 && args[0] == "scan":
		err = scan()
	case len(args) >= 2 && args[0] == "tv":
		err = tvCommand(args[1], args[2:])
	case len(args) >= 1 && args[0] == "wifi":
		err = wifiCommand(args[1:])
	case len(args) >= 2 && args[0] == "freebox":
		err = freeboxCommand(args[1], args[2:])
	default:
		fmt.Printf(usage, store.Path())
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "canape:", err)
		return 1
	}
	return 0
}

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
