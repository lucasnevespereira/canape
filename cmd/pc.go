package cmd

import (
	"errors"
	"fmt"

	"github.com/lucasnevespereira/canape/internal/computer"
)

// Words people type, mapped to the key they mean.
var pcKeys = map[string]computer.Key{
	"play": computer.PlayPause, "pause": computer.PlayPause, "playpause": computer.PlayPause,
	"space": computer.Space, "back": computer.Back, "forward": computer.Forward,
	"fullscreen": computer.Fullscreen, "full": computer.Fullscreen,
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
		return fmt.Errorf("unknown key %q. Try: play, back, forward, volume up, volume down, mute, fullscreen, esc, space", args[0])
	}
	return computer.Press(key)
}
