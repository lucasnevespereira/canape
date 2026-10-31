package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lucasnevespereira/canape/internal/computer"
)

// Words people type, mapped to the key they mean.
var pcKeys = map[string]computer.Key{
	// play and pause send the space bar: it reaches the player in front,
	// which works for web video. The media key is there for Spotify and Music.
	"play": computer.Space, "pause": computer.Space, "space": computer.Space,
	"media": computer.PlayPause, "click": computer.Click,
	"back": computer.Back, "forward": computer.Forward, "left": computer.Back, "right": computer.Forward,
	"up": computer.Up, "down": computer.Down, "enter": computer.Enter, "ok": computer.Enter,
	"fullscreen": computer.Fullscreen, "full": computer.Fullscreen, "delete": computer.Delete,
	"esc": computer.Escape, "escape": computer.Escape, "mute": computer.Mute,
}

func pcCommand(args []string) error {
	if !computer.Supported() {
		return computer.ErrUnsupported
	}
	if len(args) == 0 {
		v, err := computer.GetVolume()
		if err != nil {
			return err
		}
		fmt.Printf("%s, volume %d%s\n", computer.Name(), v.Level, map[bool]string{true: " (muted)"}[v.Muted])
		return nil
	}
	if args[0] == "type" {
		text := strings.Join(args[1:], " ")
		if text == "" {
			// Prompting keeps passwords out of the shell history.
			text = prompt(bufio.NewReader(os.Stdin), "Text to type on "+computer.Name(), "")
		}
		return computer.Type(text)
	}
	key, ok := pcKeys[args[0]]
	if args[0] == "volume" {
		switch {
		case len(args) > 1 && args[1] == "up":
			key, ok = computer.VolumeUp, true
		case len(args) > 1 && args[1] == "down":
			key, ok = computer.VolumeDown, true
		default:
			return errors.New("usage: canape pc volume up|down")
		}
	}
	if !ok {
		return fmt.Errorf("unknown key %q. Try: play, back, forward, up, down, enter, delete, type, click, volume up, volume down, mute, fullscreen, esc, media", args[0])
	}
	return computer.Press(key)
}
