// Package cmd is the canape command: argument parsing and terminal output.
package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lucasnevespereira/canape/internal/mcp"
	"github.com/lucasnevespereira/canape/internal/server"
	"github.com/lucasnevespereira/canape/internal/store"
	"github.com/lucasnevespereira/canape/internal/version"
)

const usage = `canape explains the Wi-Fi you're on and controls what's on it: the TV, this computer.

Get started
  canape setup                 pair this network's TV and router, save the Wi-Fi (safe to rerun)
  canape                       this network in plain words: Wi-Fi, box, internet, devices

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

Computer (the one canape runs on, macOS for now)
  canape pc play               play or pause the player in front (the space bar)
  canape pc click              click the video so the player listens to keys
  canape pc back | forward     skip back or ahead in the player
  canape pc media              the media key, for Spotify or Music
  canape pc volume up|down     change the computer's volume
  canape pc mute               mute or unmute the computer
  canape pc fullscreen | esc   enter or leave full screen

Wake a computer
  canape wake add              save a computer for waking (it must be on now)
  canape wake <name>           wake a saved computer
  canape wake                  list saved computers, awake or asleep
  canape wake forget <name>    stop listing it

Wi-Fi
  canape wifi                  the Wi-Fi QR code, in the terminal
  canape wifi tv               the Wi-Fi QR code, on the TV
  canape wifi set              change the saved Wi-Fi name or password

Network
  canape scan                  look around: every device on this network (saves nothing)
  canape devices [all]         who's online, from the router's own list (Freebox)
  canape homes                 every network canape knows · rename <name> · forget <name>

Run
  canape serve [addr]          web dashboard, made for your phone (default :8080)
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
		err = report()
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
	case "pc", "computer":
		err = pcCommand(rest)
	case "wake":
		err = wakeCommand(rest)
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

func row(label, value string) { fmt.Printf("  %-9s %s\n", label, value) }

// lineReader is a *bufio.Reader, or lines when a step must also be skippable.
type lineReader interface {
	ReadString(delim byte) (string, error)
}

// lines is stdin one line at a time, so a long wait can also listen for Enter.
type lines chan string

func stdinLines() lines { return readLines(os.Stdin) }

func readLines(src io.Reader) lines {
	ch := make(lines)
	go func() {
		r := bufio.NewReader(src)
		for {
			line, err := r.ReadString('\n')
			if line != "" {
				ch <- line
			}
			if err != nil {
				close(ch)
				return
			}
		}
	}()
	return ch
}

func (l lines) ReadString(byte) (string, error) {
	line, ok := <-l
	if !ok {
		return "", io.EOF
	}
	return line, nil
}

func prompt(in lineReader, label, def string) string {
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
