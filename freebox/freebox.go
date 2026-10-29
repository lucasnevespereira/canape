// Package freebox talks to the Freebox OS API on the local network.
// Docs: https://dev.freebox.fr/sdk/os/
package freebox

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ponytail: plain HTTP. The box also serves HTTPS, but with Free's own CA,
// which would have to be bundled. Fine on the LAN; bundle the CA if canape
// ever talks to the box from outside.
const host = "http://mafreebox.freebox.fr"

const AppID = "canape"

var httpClient = &http.Client{Timeout: 10 * time.Second}

// APIError is the error the box reports, e.g. auth_required or insufficient_rights.
type APIError struct {
	Code string
	Msg  string
}

func (e *APIError) Error() string { return fmt.Sprintf("freebox: %s (%s)", e.Msg, e.Code) }

// apiBase reads the API version from the box, so no version is hardcoded.
func apiBase() (string, error) {
	resp, err := httpClient.Get(host + "/api_version")
	if err != nil {
		return "", fmt.Errorf("no Freebox found: %w", err)
	}
	defer resp.Body.Close()
	var v struct {
		Base    string `json:"api_base_url"`
		Version string `json:"api_version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	major, _, _ := strings.Cut(v.Version, ".")
	return fmt.Sprintf("%s%sv%s/", host, v.Base, major), nil
}

type Client struct {
	base    string
	token   string // app_token: long-lived secret from Authorize
	session string // short-lived, renewed on auth_required
	Perms   map[string]bool
}

func New(appToken string) (*Client, error) {
	base, err := apiBase()
	if err != nil {
		return nil, err
	}
	return &Client{base: base, token: appToken}, nil
}

// Authorize asks the box for an app token. The owner has to confirm on the
// box's screen; waiting is called once the request is shown there.
func Authorize(ctx context.Context, appName, version, device string, waiting func()) (string, error) {
	base, err := apiBase()
	if err != nil {
		return "", err
	}
	c := &Client{base: base}
	var auth struct {
		Token   string `json:"app_token"`
		TrackID int    `json:"track_id"`
	}
	err = c.raw("POST", "login/authorize/", map[string]string{
		"app_id": AppID, "app_name": appName, "app_version": version, "device_name": device,
	}, &auth, false)
	if err != nil {
		return "", err
	}
	waiting()
	for {
		var st struct {
			Status string `json:"status"`
		}
		if err := c.raw("GET", fmt.Sprintf("login/authorize/%d", auth.TrackID), nil, &st, false); err != nil {
			return "", err
		}
		switch st.Status {
		case "granted":
			return auth.Token, nil
		case "pending":
		default: // denied, timeout, unknown
			return "", fmt.Errorf("authorization %s", st.Status)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// login opens a session. The password is an HMAC of the box's challenge
// keyed with the app token, so the token itself never goes on the wire.
func (c *Client) login() error {
	var ch struct {
		Challenge string `json:"challenge"`
	}
	if err := c.raw("GET", "login/", nil, &ch, false); err != nil {
		return err
	}
	mac := hmac.New(sha1.New, []byte(c.token))
	mac.Write([]byte(ch.Challenge))
	var s struct {
		Token string          `json:"session_token"`
		Perms map[string]bool `json:"permissions"`
	}
	err := c.raw("POST", "login/session/", map[string]string{
		"app_id": AppID, "password": hex.EncodeToString(mac.Sum(nil)),
	}, &s, false)
	if err != nil {
		return err
	}
	c.session, c.Perms = s.Token, s.Perms
	return nil
}

// Login opens a session right away, to check the token and read permissions.
func (c *Client) Login() error { return c.login() }

// do calls the API with a session, opening or renewing it as needed.
func (c *Client) do(method, path string, body, out any) error {
	if c.session == "" {
		if err := c.login(); err != nil {
			return err
		}
	}
	err := c.raw(method, path, body, out, true)
	var ae *APIError
	if errors.As(err, &ae) && ae.Code == "auth_required" { // session expired
		if err := c.login(); err != nil {
			return err
		}
		err = c.raw(method, path, body, out, true)
	}
	return err
}

func (c *Client) raw(method, path string, body, out any, auth bool) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("X-Fbx-App-Auth", c.session)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var env struct {
		Success   bool            `json:"success"`
		Result    json.RawMessage `json:"result"`
		ErrorCode string          `json:"error_code"`
		Msg       string          `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("freebox %s %s: %w", method, path, err)
	}
	if !env.Success {
		return &APIError{Code: env.ErrorCode, Msg: env.Msg}
	}
	if out != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// SSID returns the main Wi-Fi network name. The API masks the password
// ("********"), so that has to come from the user.
func (c *Client) SSID() (string, error) {
	var bss []struct {
		Config struct {
			SSID string `json:"ssid"`
		} `json:"config"`
	}
	if err := c.do("GET", "wifi/bss/", nil, &bss); err != nil {
		return "", err
	}
	if len(bss) == 0 {
		return "", errors.New("freebox: no Wi-Fi network found")
	}
	return bss[0].Config.SSID, nil
}

type Host struct {
	Name         string `json:"primary_name"`
	Type         string `json:"host_type"`
	Vendor       string `json:"vendor_name"`
	Reachable    bool   `json:"reachable"`
	LastActivity int64  `json:"last_activity"`
	L2           struct {
		ID string `json:"id"`
	} `json:"l2ident"`
	L3 []struct {
		Addr   string `json:"addr"`
		AF     string `json:"af"`
		Active bool   `json:"active"`
	} `json:"l3connectivities"`
}

func (h Host) MAC() string { return h.L2.ID }

// IPv4 prefers the address in use; a device keeps old ones in the list.
func (h Host) IPv4() string {
	ip := ""
	for _, l := range h.L3 {
		if l.AF == "ipv4" {
			if l.Active {
				return l.Addr
			}
			ip = l.Addr
		}
	}
	return ip
}

// Hosts lists every device the box has seen on the LAN, online ones first.
func (c *Client) Hosts() ([]Host, error) {
	var hosts []Host
	if err := c.do("GET", "lan/browser/pub/", nil, &hosts); err != nil {
		return nil, err
	}
	sort.Slice(hosts, func(i, j int) bool {
		if hosts[i].Reachable != hosts[j].Reachable {
			return hosts[i].Reachable
		}
		return strings.ToLower(hosts[i].Name) < strings.ToLower(hosts[j].Name)
	})
	return hosts, nil
}
