// Package android drives Android TVs and Android projectors (XGIMI...)
// over ADB with network debugging on. It speaks the ADB protocol itself, so
// nothing else needs installing. Notes in docs/android.md.
package android

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/tv"
)

const port = "5555"

type TV struct{ adb *adb }

var _ tv.TV = (*TV)(nil)

// Connect dials the TV and signs in with key, the private key saved at
// pairing. With an empty key canape makes one, and the TV asks the user to
// allow debugging from this computer. The key to save comes back.
func Connect(ctx context.Context, ip, key string, prompted func()) (*TV, string, error) {
	var k *rsa.PrivateKey
	var err error
	if key == "" {
		k, key, err = newKey()
	} else {
		k, err = parseKey(key)
	}
	if err != nil {
		return nil, "", err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, port))
	if err != nil {
		return nil, "", fmt.Errorf("connect to TV at %s: %w", ip, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	// Like LG's prompt, the user may need a while to find the remote.
	a, err := handshake(conn, k, prompted, 60*time.Second)
	if err != nil {
		conn.Close()
		return nil, "", err
	}
	return &TV{adb: a}, key, nil
}

func (t *TV) Close() error { return t.adb.Close() }

func (t *TV) shell(cmd string) (string, error) { return t.adb.Shell(cmd) }

func (t *TV) keyevent(code int) error {
	_, err := t.shell("input keyevent " + strconv.Itoa(code))
	return err
}

// Android keycodes, from android.view.KeyEvent.
var keycodes = map[string]int{
	tv.KeyHome:  3,
	tv.KeyBack:  4,
	tv.KeyUp:    19,
	tv.KeyDown:  20,
	tv.KeyLeft:  21,
	tv.KeyRight: 22,
	tv.KeyOK:    23,
}

func (t *TV) Key(name string) error {
	code, ok := keycodes[strings.ToUpper(name)]
	if !ok {
		return fmt.Errorf("unknown key %q", name)
	}
	return t.keyevent(code)
}

// TurnOff sends the sleep key, which puts the TV in standby rather than
// toggling like the power key does.
func (t *TV) TurnOff() error { return t.keyevent(223) }

// music reads the media stream from dumpsys audio: its level, maximum and
// mute. Volume tools print nothing on some Android 11 firmware, this does.
func (t *TV) music() (level, top int, muted bool, err error) {
	out, err := t.shell("dumpsys audio | grep -A6 '\\- STREAM_MUSIC:'")
	if err != nil {
		return 0, 0, false, err
	}
	level, top, muted, ok := parseMusic(out)
	if !ok {
		// ponytail: streamVolume shows up from Android 11. Read the "Current:" line if an Android 10 TV lacks it.
		return 0, 0, false, fmt.Errorf("can't read the volume: %q", strings.TrimSpace(out))
	}
	return level, top, muted, nil
}

var (
	levelRe = regexp.MustCompile(`streamVolume:(\d+)`)
	maxRe   = regexp.MustCompile(`Max: (\d+)`)
)

func parseMusic(out string) (level, top int, muted, ok bool) {
	l, m := levelRe.FindStringSubmatch(out), maxRe.FindStringSubmatch(out)
	if l == nil || m == nil {
		return 0, 0, false, false
	}
	level, _ = strconv.Atoi(l[1])
	top, _ = strconv.Atoi(m[1])
	return level, top, strings.Contains(out, "Muted: true"), top > 0
}

func (t *TV) Volume() (tv.Volume, error) {
	level, top, muted, err := t.music()
	return tv.Volume{Level: level * 100 / max(top, 1), Muted: muted}, err
}

// SetVolume uses `cmd media_session` (Android 11 and later), or `media` on
// Android 10, which had it as its own tool.
func (t *TV) SetVolume(level int) error {
	_, top, _, err := t.music()
	if err != nil {
		return err
	}
	n := strconv.Itoa((min(max(level, 0), 100)*top + 50) / 100)
	_, err = t.shell("(cmd media_session volume --stream 3 --set " + n + " || media volume --stream 3 --set " + n + ") 2>/dev/null")
	return err
}

// SetMute presses the mute key only when needed: Android has no "set mute"
// command in its shell, only a key that toggles.
func (t *TV) SetMute(mute bool) error {
	v, err := t.Volume()
	if err != nil {
		return err
	}
	if v.Muted == mute {
		return nil
	}
	return t.keyevent(164)
}

// ponytail: no HDMI inputs yet. Android lists them through TvInputManager, add it when a TV box on HDMI needs it.
func (t *TV) Inputs() ([]tv.Input, error) { return nil, nil }
func (t *TV) SwitchInput(id string) error { return tv.ErrUnsupported }

// Notify isn't possible from the shell: TV launchers don't show
// notifications posted by `cmd notification`.
func (t *TV) Notify(message string) error { return tv.ErrUnsupported }

// Apps lists what the TV launcher shows: apps with a Leanback launcher
// activity. The shell can't read app labels, so known ones get a title and
// the rest show their package name.
func (t *TV) Apps() ([]tv.App, error) {
	out, err := t.shell("cmd package query-activities --brief -a android.intent.action.MAIN -c android.intent.category.LEANBACK_LAUNCHER")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var apps []tv.App
	for _, line := range strings.Split(out, "\n") {
		pkg, _, ok := strings.Cut(strings.TrimSpace(line), "/")
		if !ok || seen[pkg] {
			continue
		}
		seen[pkg] = true
		apps = append(apps, tv.App{ID: pkg, Title: title(pkg)})
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Title) < strings.ToLower(apps[j].Title) })
	return apps, nil
}

var titles = map[string]string{
	"com.google.android.youtube.tv":     "YouTube",
	"com.teamsmart.videomanager.tv":     "SmartTube",
	"com.netflix.ninja":                 "Netflix",
	"com.amazon.amazonvideo.livingroom": "Prime Video",
	"com.disney.disneyplus":             "Disney+",
	"com.canal.android.canal":           "myCANAL",
	"com.plexapp.android":               "Plex",
	"org.xbmc.kodi":                     "Kodi",
	"org.videolan.vlc":                  "VLC",
	"com.spotify.tv.android":            "Spotify",
	"com.android.vending":               "Play Store",
	"com.android.tv.settings":           "Settings",
}

func title(pkg string) string {
	if t, ok := titles[pkg]; ok {
		return t
	}
	return pkg
}

// Launch opens an app the way the launcher does.
func (t *TV) Launch(appID string) error {
	out, err := t.shell("monkey -p " + shellQuote(appID) + " -c android.intent.category.LEANBACK_LAUNCHER 1")
	if err == nil && strings.Contains(out, "No activities found") {
		err = fmt.Errorf("can't open %s", appID)
	}
	return err
}

// Current returns the package of the app on screen.
func (t *TV) Current() (string, error) {
	out, err := t.shell("dumpsys activity activities | grep -m1 mResumedActivity")
	if err != nil {
		return "", err
	}
	// mResumedActivity: ActivityRecord{1a2b u0 com.netflix.ninja/.MainActivity t12}
	for _, f := range strings.Fields(out) {
		if pkg, _, ok := strings.Cut(f, "/"); ok {
			return pkg, nil
		}
	}
	return "", errors.New("can't tell what's on screen")
}

func (t *TV) Open(url string) error {
	_, err := t.shell("am start -a android.intent.action.VIEW -d " + shellQuote(url))
	return err
}

// Type types into the focused field. `input text` reads %s as a space.
func (t *TV) Type(text string) error {
	_, err := t.shell("input text " + shellQuote(strings.ReplaceAll(text, " ", "%s")))
	return err
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Discover returns the first device on this /24 with ADB listening, which
// only happens once network debugging is on.
func Discover(timeout time.Duration) (string, error) {
	local := discovery.LocalIP()
	i := strings.LastIndex(local, ".")
	if i < 0 {
		return "", errors.New("no network connection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	found := make(chan string, 1)
	for n := 1; n < 255; n++ {
		ip := local[:i+1] + strconv.Itoa(n)
		if ip == local {
			continue
		}
		go func() {
			var d net.Dialer
			if c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, port)); err == nil {
				c.Close()
				select {
				case found <- ip:
				default:
				}
			}
		}()
	}
	select {
	case ip := <-found:
		return ip, nil
	case <-ctx.Done():
		return "", errors.New("no Android TV with network debugging answered. See docs/android.md to turn it on")
	}
}
