// Package lg talks to LG webOS TVs over their local WebSocket API (SSAP).
package lg

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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/lucasnevespereira/canape/internal/tv"
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

var _ tv.TV = (*TV)(nil)

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

// Connect dials the TV and registers with key. With an empty or forgotten
// key the TV asks the user to accept canape; the key to save comes back.
func Connect(ctx context.Context, ip, key string, prompted func()) (*TV, string, error) {
	t, err := Dial(ctx, ip)
	if err != nil {
		return nil, "", err
	}
	if key, err = t.Register(key, prompted); err != nil {
		t.Close()
		return nil, "", err
	}
	return t, key, nil
}

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

func (t *TV) Notify(message string) error {
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

// Type inserts text into the field that has focus, as if typed on the
// on-screen keyboard. Nothing happens if no text field is focused.
func (t *TV) Type(text string) error {
	return t.do("com.webos.service.ime/insertText", map[string]any{"text": text, "replace": 0})
}

// Open shows a URL in the TV's web browser.
func (t *TV) Open(url string) error {
	return t.do("system.launcher/open", map[string]string{"target": url})
}

// Key presses a remote key: tv.Keys, which are LG's own names. Keys don't go
// through SSAP: the TV hands out a second socket that takes plain-text frames.
func (t *TV) Key(name string) error {
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

func (t *TV) Volume() (tv.Volume, error) {
	// ponytail: shape from webOS 4. Newer firmware may nest these, handle it when a newer TV shows up.
	var v struct {
		Level int  `json:"volume"`
		Muted bool `json:"muted"`
	}
	out, err := t.Call("audio/getVolume", nil)
	if err != nil {
		return tv.Volume{}, err
	}
	err = json.Unmarshal(out, &v)
	return tv.Volume{Level: v.Level, Muted: v.Muted}, err
}

// input is the TV's own description: each input is also an app, which is
// what the foreground app reports when it's on screen.
type input struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	AppID     string `json:"appId"`
	Connected bool   `json:"connected"`
}

func (t *TV) inputs() ([]input, error) {
	out, err := t.Call("tv/getExternalInputList", nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Devices []input `json:"devices"`
	}
	return r.Devices, json.Unmarshal(out, &r)
}

func (t *TV) Inputs() ([]tv.Input, error) {
	ins, err := t.inputs()
	var out []tv.Input
	for _, in := range ins {
		if strings.HasPrefix(in.ID, "HDMI") { // AV and component inputs are legacy noise
			out = append(out, tv.Input{ID: in.ID, Label: in.Label, Connected: in.Connected})
		}
	}
	return out, err
}

// Apps returns the apps shown in the launcher, sorted by title. The TV has
// about four times as many hidden system apps.
func (t *TV) Apps() ([]tv.App, error) {
	out, err := t.Call("com.webos.applicationManager/listApps", nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Apps []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Visible bool   `json:"visible"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, err
	}
	var apps []tv.App
	for _, a := range r.Apps {
		if a.Visible {
			apps = append(apps, tv.App{ID: a.ID, Title: a.Title})
		}
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Title) < strings.ToLower(apps[j].Title) })
	return apps, nil
}

// Current returns the app on screen, or the input ID (HDMI_1) when an
// input is: LG reports inputs as apps (com.webos.app.hdmi1).
func (t *TV) Current() (string, error) {
	out, err := t.Call("com.webos.applicationManager/getForegroundAppInfo", nil)
	if err != nil {
		return "", err
	}
	var r struct {
		AppID string `json:"appId"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", err
	}
	ins, _ := t.inputs()
	for _, in := range ins {
		if in.AppID == r.AppID {
			return in.ID, nil
		}
	}
	return r.AppID, nil
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
