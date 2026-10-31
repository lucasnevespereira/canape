// Package computer presses keys on the machine canape runs on, so a phone can
// pause or skip the movie that machine is streaming to the TV.
package computer

import (
	"errors"
	"os"
	"runtime"
	"strings"
)

type Key string

const (
	Space      Key = "space"       // plays and pauses the player in front, the most reliable for web video
	PlayPause  Key = "playpause"   // the media key: goes to what macOS thinks is playing, often Music or Spotify
	Click      Key = "click"       // clicks the middle of the screen so a web player gets focus
	Back       Key = "back"        // left arrow: most players skip back 5 or 10 seconds
	Forward    Key = "forward"     // right arrow
	Up         Key = "up"          // arrows up and down, for menus and lists
	Down       Key = "down"        //
	Enter      Key = "enter"       //
	Delete     Key = "delete"      // backspace, to fix what was typed
	Fullscreen Key = "fullscreen"  // F
	Escape     Key = "escape"      // leaves fullscreen
	VolumeUp   Key = "volume-up"   // the computer's own volume
	VolumeDown Key = "volume-down" //
	Mute       Key = "mute"        // toggles
)

var Keys = []Key{Space, PlayPause, Click, Back, Forward, Up, Down, Enter, Delete, Fullscreen, Escape, VolumeUp, VolumeDown, Mute}

var ErrUnsupported = errors.New("controlling this computer isn't supported on " + runtime.GOOS + " yet")

// ErrPermission means macOS wants canape allowed under Accessibility first.
// macOS shows its own prompt the first time.
var ErrPermission = errors.New("macOS needs your permission first: System Settings > Privacy & Security > Accessibility, then allow the app canape runs in (Terminal, or the app you started canape from)")

type Volume struct {
	Level int  `json:"level"`
	Muted bool `json:"muted"`
}

// hostname is the fallback name. Routers often hand out unreadable ones.
func hostname() string {
	name, _ := os.Hostname()
	return strings.TrimSuffix(name, ".local")
}

// Supported says whether Press works on this system.
func Supported() bool { return runtime.GOOS == "darwin" }
