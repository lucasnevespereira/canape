// Package lgtv talks to LG webOS TVs over their local WebSocket API (SSAP).
package lgtv

import (
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// register.json is the pairing request open-source LG clients send, taken
// from PyWebOSTV. Its signature covers the "signed" block, so edit neither.
//
//go:embed register.json
var registerJSON []byte

type TV struct {
	conn *websocket.Conn
	next int
}

type message struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	URI     string          `json:"uri,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// No Origin header: the TV closes connections that send one (1008 invalid origin).
var dialOpts = &websocket.DialOptions{HTTPClient: &http.Client{Transport: &http.Transport{
	// ponytail: the TV's certificate is self-signed, so it isn't verified. Pin the one seen at pairing if that matters.
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}}}

// Dial connects to the TV. It tries the TLS port first because recent
// firmware closes the plain one.
func Dial(ctx context.Context, ip string) (*TV, error) {
	var errs []error
	for _, u := range []string{"wss://" + ip + ":3001", "ws://" + ip + ":3000"} {
		conn, _, err := websocket.Dial(ctx, u, dialOpts)
		if err == nil {
			conn.SetReadLimit(4 << 20) // app and input lists outgrow the 32 KB default
			return &TV{conn: conn}, nil
		}
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("connect to TV at %s: %w", ip, errors.Join(errs...))
}

func (t *TV) Close() error { return t.conn.Close(websocket.StatusNormalClosure, "") }

// Register authenticates with the client key from an earlier pairing. With
// no key, or one the TV forgot, the TV shows an accept prompt and prompted
// is called while it waits. It returns the key to store.
func (t *TV) Register(key string, prompted func()) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(registerJSON, &payload); err != nil {
		return "", err
	}
	if key != "" {
		payload["client-key"] = key
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := t.send(ctx, "register", "register", "", payload); err != nil {
		return "", err
	}
	for {
		var m message
		if err := wsjson.Read(ctx, t.conn, &m); err != nil {
			return "", fmt.Errorf("waiting for pairing: %w", err)
		}
		switch m.Type {
		case "registered":
			var p struct {
				Key string `json:"client-key"`
			}
			err := json.Unmarshal(m.Payload, &p)
			return p.Key, err
		case "error":
			return "", fmt.Errorf("TV refused pairing: %s", m.Error)
		case "response":
			if prompted != nil && bytes.Contains(m.Payload, []byte(`"PROMPT"`)) {
				prompted()
			}
		}
	}
}

// Call sends one request and returns the payload of the TV's reply. The
// "ssap://" prefix of uri is optional.
func (t *TV) Call(uri string, payload any) (json.RawMessage, error) {
	if !strings.Contains(uri, "://") {
		uri = "ssap://" + uri
	}
	t.next++
	id := strconv.Itoa(t.next)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := t.send(ctx, "request", id, uri, payload); err != nil {
		return nil, err
	}
	for {
		var m message
		if err := wsjson.Read(ctx, t.conn, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", uri, err)
		}
		if m.ID != id {
			continue
		}
		if m.Type == "error" {
			return nil, fmt.Errorf("%s: %s", uri, m.Error)
		}
		var r struct {
			OK   *bool  `json:"returnValue"`
			Text string `json:"errorText"`
		}
		json.Unmarshal(m.Payload, &r)
		if r.OK != nil && !*r.OK {
			return m.Payload, fmt.Errorf("%s: %s", uri, r.Text)
		}
		return m.Payload, nil
	}
}

func (t *TV) TurnOff() error { return t.do("system/turnOff", nil) }

func (t *TV) Toast(message string) error {
	return t.do("system.notifications/createToast", map[string]string{"message": message})
}

func (t *TV) SetVolume(level int) error {
	return t.do("audio/setVolume", map[string]int{"volume": min(max(level, 0), 100)})
}

func (t *TV) SetMute(mute bool) error { return t.do("audio/setMute", map[string]bool{"mute": mute}) }

func (t *TV) SwitchInput(id string) error {
	return t.do("tv/switchInput", map[string]string{"inputId": id})
}

func (t *TV) Launch(appID string) error {
	return t.do("system.launcher/launch", map[string]string{"id": appID})
}

// Open shows a URL in the TV's web browser.
func (t *TV) Open(url string) error {
	return t.do("system.launcher/open", map[string]string{"target": url})
}

// Button presses a remote key: HOME, BACK, UP, DOWN, LEFT, RIGHT, ENTER...
// Keys don't go through SSAP: the TV hands out a second socket that takes
// plain-text frames.
func (t *TV) Button(name string) error {
	out, err := t.Call("com.webos.service.networkinput/getPointerInputSocket", nil)
	if err != nil {
		return err
	}
	var r struct {
		Path string `json:"socketPath"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, r.Path, dialOpts)
	if err != nil {
		return fmt.Errorf("button socket: %w", err)
	}
	// CloseNow skips the close handshake, which the TV may never answer.
	defer conn.CloseNow()
	return conn.Write(ctx, websocket.MessageText, []byte("type:button\nname:"+strings.ToUpper(name)+"\n\n"))
}

func (t *TV) do(uri string, payload any) error {
	_, err := t.Call(uri, payload)
	return err
}

// ponytail: shape from webOS 4. Newer firmware may nest these, handle it when a newer TV shows up.
type Volume struct {
	Level int  `json:"volume"`
	Muted bool `json:"muted"`
}

func (t *TV) Volume() (Volume, error) {
	var v Volume
	out, err := t.Call("audio/getVolume", nil)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(out, &v)
}

type Input struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	AppID     string `json:"appId"`
	Connected bool   `json:"connected"`
}

func (t *TV) Inputs() ([]Input, error) {
	out, err := t.Call("tv/getExternalInputList", nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Devices []Input `json:"devices"`
	}
	return r.Devices, json.Unmarshal(out, &r)
}

type App struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Visible bool   `json:"visible"`
}

// Apps returns the apps shown in the launcher, sorted by title. The TV has
// about four times as many hidden system apps.
func (t *TV) Apps() ([]App, error) {
	out, err := t.Call("com.webos.applicationManager/listApps", nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Apps []App `json:"apps"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, err
	}
	var apps []App
	for _, a := range r.Apps {
		if a.Visible {
			apps = append(apps, a)
		}
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Title) < strings.ToLower(apps[j].Title) })
	return apps, nil
}

// ForegroundApp returns the app on screen. An input counts as an app,
// e.g. com.webos.app.hdmi1.
func (t *TV) ForegroundApp() (string, error) {
	out, err := t.Call("com.webos.applicationManager/getForegroundAppInfo", nil)
	if err != nil {
		return "", err
	}
	var r struct {
		AppID string `json:"appId"`
	}
	return r.AppID, json.Unmarshal(out, &r)
}

func (t *TV) send(ctx context.Context, typ, id, uri string, payload any) error {
	m := message{Type: typ, ID: id, URI: uri}
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		m.Payload = b
	}
	return wsjson.Write(ctx, t.conn, m)
}

// Discover returns the IP of the first LG TV that answers an SSDP search.
func Discover(timeout time.Duration) (string, error) {
	c, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return "", err
	}
	defer c.Close()
	msg := []byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: urn:lge-com:service:webos-second-screen:1\r\n\r\n")
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	for range 3 { // UDP drops packets
		c.WriteToUDP(msg, dst)
	}
	c.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 4096)
	for {
		n, addr, err := c.ReadFromUDP(buf)
		if err != nil {
			return "", errors.New("no LG TV answered. Is it on and on the same network?")
		}
		// Some devices answer every search, whatever was asked.
		if bytes.Contains(buf[:n], []byte("webos-second-screen")) {
			return addr.IP.String(), nil
		}
	}
}

// Wake sends a Wake-on-LAN magic packet: 6 bytes of 0xFF then the MAC 16
// times. The TV only listens for it with "Turn on via Wi-Fi" enabled.
func Wake(mac, ip string) error {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return err
	}
	pkt := bytes.Repeat([]byte{0xff}, 6)
	for range 16 {
		pkt = append(pkt, hw...)
	}
	// ponytail: the subnet broadcast assumes a /24.
	targets := []string{"255.255.255.255:9", ip[:strings.LastIndex(ip, ".")] + ".255:9"}
	var errs []error
	for _, a := range targets {
		c, err := net.Dial("udp", a)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_, err = c.Write(pkt)
		c.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == len(targets) {
		return fmt.Errorf("send wake packet: %w", errors.Join(errs...))
	}
	return nil
}

var macRe = regexp.MustCompile(`([0-9a-fA-F]{1,2}:){5}[0-9a-fA-F]{1,2}`)

// MAC reads a device's MAC from the ARP cache, which is warm right after
// talking to it. Empty if not found.
func MAC(ip string) string {
	for _, cmd := range [][]string{{"arp", "-n", ip}, {"ip", "neigh", "show", ip}} {
		out, err := exec.Command(cmd[0], cmd[1:]...).Output()
		m := macRe.FindString(string(out))
		if err != nil || m == "" {
			continue
		}
		// macOS prints "aa:bb:cc:d:ee:ff" for "aa:bb:cc:0d:ee:ff".
		parts := strings.Split(strings.ToLower(m), ":")
		for i, p := range parts {
			if len(p) == 1 {
				parts[i] = "0" + p
			}
		}
		return strings.Join(parts, ":")
	}
	return ""
}
