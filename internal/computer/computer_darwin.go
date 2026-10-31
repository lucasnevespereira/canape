package computer

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// keyScript posts key events through JavaScript for Automation, which ships
// with macOS, so canape needs no cgo. Posting events needs the Accessibility
// permission. Without it the script asks macOS to show its prompt.
const keyScript = `
ObjC.import("Cocoa");
ObjC.import("ApplicationServices");
function run(argv) {
  if (!$.AXIsProcessTrusted()) {
    $.AXIsProcessTrustedWithOptions($({ AXTrustedCheckOptionPrompt: true }));
    return "untrusted";
  }
  if (argv[0] === "click") {
    // The middle of the main screen, where a playing video usually is.
    const f = $.NSScreen.mainScreen.frame;
    const at = $.NSMakePoint(f.size.width / 2, f.size.height / 2);
    for (const type of [1, 2]) { // left mouse down, then up
      $.CGEventPost(0, $.CGEventCreateMouseEvent(null, type, at, 0));
    }
    return "ok";
  }
  if (argv[0] === "text") {
    // One event per character carrying the character itself, so any keyboard
    // layout and accents work. A Ref holds one UTF-16 unit, so no emoji.
    const s = argv[1];
    for (let i = 0; i < s.length; i++) {
      const ch = Ref("S");
      ch[0] = s.charCodeAt(i);
      for (const down of [true, false]) {
        const ev = $.CGEventCreateKeyboardEvent(null, 0, down);
        $.CGEventKeyboardSetUnicodeString(ev, 1, ch);
        $.CGEventPost(0, ev);
      }
    }
    return "ok";
  }
  const code = parseInt(argv[1], 10);
  for (const down of [true, false]) {
    if (argv[0] === "media") {
      // Media keys are "system defined" events, not keyboard events.
      const ev = $.NSEvent.otherEventWithTypeLocationModifierFlagsTimestampWindowNumberContextSubtypeData1Data2(
        14, $.NSMakePoint(0, 0), down ? 0xa00 : 0xb00, 0, 0, null, 8, (code << 16) | ((down ? 0xa : 0xb) << 8), -1);
      $.CGEventPost(0, ev.CGEvent);
    } else {
      $.CGEventPost(0, $.CGEventCreateKeyboardEvent(null, code, down));
    }
  }
  return "ok";
}`

// macOS virtual key codes, and NX_KEYTYPE_PLAY for the media key.
var keys = map[Key][2]string{
	PlayPause:  {"media", "16"},
	Click:      {"click", "0"},
	Space:      {"key", "49"},
	Back:       {"key", "123"},
	Forward:    {"key", "124"},
	Up:         {"key", "126"},
	Down:       {"key", "125"},
	Enter:      {"key", "36"},
	Delete:     {"key", "51"},
	Fullscreen: {"key", "3"},
	Escape:     {"key", "53"},
}

// Press sends one key to the computer. Volume goes through AppleScript, which
// needs no permission, so it works before Accessibility is allowed.
func Press(k Key) error {
	switch k {
	case VolumeUp:
		return applescript("set volume output volume ((output volume of (get volume settings)) + 6)")
	case VolumeDown:
		return applescript("set volume output volume ((output volume of (get volume settings)) - 6)")
	case Mute:
		return applescript("set volume output muted (not (output muted of (get volume settings)))")
	}
	spec, ok := keys[k]
	if !ok {
		return fmt.Errorf("unknown key %q", k)
	}
	return post(spec[0], spec[1])
}

// Type types text into the app in front, like a search box in the browser.
func Type(text string) error { return post("text", text) }

func post(kind, arg string) error {
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", keyScript, kind, arg).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) == "untrusted" {
		return ErrPermission
	}
	return nil
}

// Name is the computer's name from Sharing settings, e.g. "Lucas's MacBook Pro".
func Name() string {
	if out, err := exec.Command("scutil", "--get", "ComputerName").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	return hostname()
}

func GetVolume() (Volume, error) {
	out, err := exec.Command("osascript", "-e", "set s to get volume settings", "-e", "(output volume of s as text) & \",\" & (output muted of s as text)").Output()
	if err != nil {
		return Volume{}, err
	}
	level, muted, _ := strings.Cut(strings.TrimSpace(string(out)), ",")
	n, _ := strconv.Atoi(level)
	return Volume{Level: n, Muted: muted == "true"}, nil
}

func applescript(script string) error {
	if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
