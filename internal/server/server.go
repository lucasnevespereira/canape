// Package server is the web dashboard: an embedded page, a small JSON
// API, and the public Wi-Fi page TVs open.
package server

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/lucasnevespereira/canape/internal/computer"
	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/qrcode"
	"github.com/lucasnevespereira/canape/internal/tv"
	"github.com/lucasnevespereira/canape/internal/wifi"
)

//go:embed index.html
var indexHTML []byte

type server struct {
	// ponytail: one lock for every TV request. It keeps one socket open at a
	// time and guards the state file, which ConnectTV may rewrite. Fine for a
	// household. Split it if requests ever queue up.
	mu      sync.Mutex
	key     string // the login key
	baseURL string // how the TV reaches this server
}

func Run(addr string) error {
	h, err := home.Load()
	if err != nil {
		return err
	}
	// The login key covers every home, so it lives outside them.
	if h.State.Dashboard.Key == "" {
		b := make([]byte, 16)
		rand.Read(b)
		h.State.Dashboard.Key = hex.EncodeToString(b)
		if err := h.State.Save(); err != nil {
			return err
		}
	}
	port := addr[strings.LastIndex(addr, ":")+1:]
	sv := &server{key: h.State.Dashboard.Key, baseURL: "http://" + discovery.LocalIP() + ":" + port}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", sv.index)
	// The Wi-Fi page is public: the TV's browser has no login cookie, and
	// anyone who can reach this server is already on the Wi-Fi.
	mux.HandleFunc("GET /wifi", wifiPage)
	mux.HandleFunc("GET /wifi.svg", wifiSVG)
	mux.HandleFunc("GET /manifest.webmanifest", sv.manifest)
	mux.HandleFunc("GET /icons/{file}", icon)
	mux.HandleFunc("GET /api/home", sv.auth(homeInfo))
	mux.HandleFunc("GET /api/devices", sv.auth(devices))
	mux.HandleFunc("GET /api/computer", sv.auth(computerInfo))
	mux.HandleFunc("POST /api/computer/key", sv.auth(computerKey))
	mux.HandleFunc("POST /api/wifi/tv", sv.auth(tvHandler(sv, func(t tv.TV, _ struct{}) error {
		return t.Open(sv.baseURL + "/wifi")
	})))
	mux.HandleFunc("GET /api/tv", sv.auth(sv.tvStatus))
	mux.HandleFunc("POST /api/tv/power", sv.auth(sv.tvPower))
	mux.HandleFunc("POST /api/tv/volume", sv.auth(tvHandler(sv, func(t tv.TV, r struct {
		Volume int `json:"volume"`
	}) error {
		return t.SetVolume(r.Volume)
	})))
	mux.HandleFunc("POST /api/tv/mute", sv.auth(tvHandler(sv, func(t tv.TV, r struct {
		Mute bool `json:"mute"`
	}) error {
		return t.SetMute(r.Mute)
	})))
	mux.HandleFunc("POST /api/tv/input", sv.auth(tvHandler(sv, func(t tv.TV, r struct {
		ID string `json:"id"`
	}) error {
		return t.SwitchInput(r.ID)
	})))
	mux.HandleFunc("POST /api/tv/key", sv.auth(tvHandler(sv, func(t tv.TV, r struct {
		Key string `json:"key"`
	}) error {
		return t.Key(strings.ToUpper(r.Key))
	})))
	mux.HandleFunc("POST /api/tv/type", sv.auth(tvHandler(sv, func(t tv.TV, r struct {
		Text string `json:"text"`
	}) error {
		return t.Type(r.Text)
	})))
	mux.HandleFunc("POST /api/tv/app", sv.auth(tvHandler(sv, func(t tv.TV, r struct {
		ID string `json:"id"`
	}) error {
		return t.Launch(r.ID)
	})))

	link := sv.baseURL + "/?key=" + h.State.Dashboard.Key
	fmt.Printf("canape is running for home %q.\n\nScan with your phone to open the web dashboard:\n\n", h.Name)
	if code, err := qrcode.Terminal(link); err == nil {
		fmt.Print(code)
	}
	fmt.Printf("\nOr open %s on a device on the same Wi-Fi.\n", link)
	fmt.Println("The link logs the device in, so only use it on your own devices. Ctrl+C to stop.")
	return http.ListenAndServe(addr, mux)
}

// index serves the page. Opening it with ?key= logs the device in: the key
// moves into a cookie and the redirect drops it from the address bar.
func (sv *server) index(w http.ResponseWriter, r *http.Request) {
	if k := r.URL.Query().Get("key"); k != "" {
		if !sv.validKey(k) {
			http.Error(w, "Wrong key. Use the link canape serve printed.", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "canape",
			Value:    k,
			Path:     "/",
			MaxAge:   400 * 24 * 3600, // the longest browsers allow
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

// The Wi-Fi handlers re-read the state file, so `canape wifi set` shows up
// without restarting the server.
func wifiPage(w http.ResponseWriter, r *http.Request) {
	h, err := home.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := wifi.Page(w, h.Wifi.SSID, h.Wifi.Password); err != nil {
		log.Print(err)
	}
}

func wifiSVG(w http.ResponseWriter, r *http.Request) {
	h, err := home.Load()
	if err != nil || h.Wifi.SSID == "" {
		http.NotFound(w, r)
		return
	}
	svg, err := qrcode.SVG(wifi.QRText(h.Wifi.SSID, h.Wifi.Password))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, svg)
}

// homeInfo is what the dashboard needs to draw its tabs: which home this
// is, what's paired, and the Wi-Fi to share. It re-reads the state file, so
// `canape setup` and `canape wifi set` show up without a restart.
func homeInfo(w http.ResponseWriter, r *http.Request) {
	h, err := home.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"name":     h.Name,
		"tv":       h.TV.Key != "",
		"tvBrand":  home.BrandName(h.TV.Brand),
		"router":   h.Freebox.AppToken != "",
		"ssid":     h.Wifi.SSID,
		"password": h.Wifi.Password, // already public on /wifi, for guests
	})
}

func devices(w http.ResponseWriter, r *http.Request) {
	h, err := home.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	hosts, err := h.Devices(false)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	type device struct {
		Name   string `json:"name"`
		IP     string `json:"ip"`
		Vendor string `json:"vendor"`
	}
	list := []device{}
	for _, d := range hosts {
		list = append(list, device{d.Name, d.IPv4(), d.Vendor})
	}
	writeJSON(w, list)
}

// computerInfo describes the machine canape runs on, the one streaming to
// the TV. The dashboard hides its Computer section when Supported is false.
func computerInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{"supported": computer.Supported(), "name": computer.Name()}
	if v, err := computer.GetVolume(); err == nil {
		info["volume"], info["muted"] = v.Level, v.Muted
	}
	writeJSON(w, info)
}

func computerKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key computer.Key `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !slices.Contains(computer.Keys, req.Key) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown key %q", req.Key))
		return
	}
	if err := computer.Press(req.Key); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	computerInfo(w, r) // the page shows the new volume
}

// manifest makes "Add to Home Screen" open canape full screen with its icon.
// For a logged-in browser, start_url carries the login key: on iPhone a
// home-screen app may not share Safari's cookies, so it logs itself in on
// first launch instead of being stuck logged out.
func (sv *server) manifest(w http.ResponseWriter, r *http.Request) {
	start := "/"
	if c, err := r.Cookie("canape"); err == nil && sv.validKey(c.Value) {
		start = "/?key=" + c.Value
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	json.NewEncoder(w).Encode(map[string]any{
		"name":             "canape",
		"short_name":       "canape",
		"start_url":        start,
		"display":          "standalone",
		"background_color": "#111214",
		"theme_color":      "#111214",
		"icons": []map[string]string{
			{"src": "/icons/192.png", "sizes": "192x192", "type": "image/png"},
			{"src": "/icons/512.png", "sizes": "512x512", "type": "image/png"},
		},
	})
}

func (sv *server) validKey(k string) bool {
	return subtle.ConstantTimeCompare([]byte(k), []byte(sv.key)) == 1
}

func (sv *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("canape")
		if err != nil || !sv.validKey(c.Value) {
			writeError(w, http.StatusUnauthorized, errors.New("not logged in"))
			return
		}
		next(w, r)
	}
}

func (sv *server) tvStatus(w http.ResponseWriter, r *http.Request) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	h, err := home.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	t, err := h.ConnectTV()
	if err != nil {
		// Off and unreachable look the same from here. The page offers power on.
		writeJSON(w, home.TVStatus{})
		return
	}
	defer t.Close()

	st, err := home.Status(t)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, st)
}

func (sv *server) tvPower(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !req.On {
		tvHandler(sv, func(t tv.TV, _ struct{}) error { return t.TurnOff() })(w, r)
		return
	}
	h, err := home.Load()
	if err == nil {
		err = h.WakeTV()
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// tvHandler decodes the JSON body into T, connects to the TV and runs fn. An
// empty or already-read body leaves T at its zero value.
func tvHandler[T any](sv *server, fn func(tv.TV, T) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req T
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		sv.mu.Lock()
		defer sv.mu.Unlock()
		h, err := home.Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		t, err := h.ConnectTV()
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		defer t.Close()
		if err := fn(t, req); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	log.Printf("%d: %v", code, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
