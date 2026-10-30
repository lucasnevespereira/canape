// Package server is the phone dashboard: an embedded page, a small JSON
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
	"strings"
	"sync"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/tv/lg"
	"github.com/lucasnevespereira/canape/internal/wifi"
)

//go:embed index.html
var indexHTML []byte

type server struct {
	// ponytail: one lock for every TV request. It keeps one socket open at a
	// time and guards the state file, which ConnectTV may rewrite. Fine for a
	// household; split it if requests ever queue up.
	mu      sync.Mutex
	here    *home.Here
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
	sv := &server{here: h, baseURL: "http://" + discovery.LocalIP() + ":" + port}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", sv.index)
	// The Wi-Fi page is public: the TV's browser has no login cookie, and
	// anyone who can reach this server is already on the Wi-Fi.
	mux.HandleFunc("GET /wifi", wifiPage)
	mux.HandleFunc("GET /wifi.svg", wifiSVG)
	mux.HandleFunc("GET /api/wifi", sv.auth(func(w http.ResponseWriter, r *http.Request) {
		h, err := home.Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]string{"ssid": h.Wifi.SSID})
	}))
	mux.HandleFunc("POST /api/wifi/tv", sv.auth(tvHandler(sv, func(tv *lg.TV, _ struct{}) error {
		return tv.Open(sv.baseURL + "/wifi")
	})))
	mux.HandleFunc("GET /api/tv", sv.auth(sv.tvStatus))
	mux.HandleFunc("POST /api/tv/power", sv.auth(sv.tvPower))
	mux.HandleFunc("POST /api/tv/volume", sv.auth(tvHandler(sv, func(tv *lg.TV, r struct {
		Volume int `json:"volume"`
	}) error {
		return tv.SetVolume(r.Volume)
	})))
	mux.HandleFunc("POST /api/tv/mute", sv.auth(tvHandler(sv, func(tv *lg.TV, r struct {
		Mute bool `json:"mute"`
	}) error {
		return tv.SetMute(r.Mute)
	})))
	mux.HandleFunc("POST /api/tv/input", sv.auth(tvHandler(sv, func(tv *lg.TV, r struct {
		ID string `json:"id"`
	}) error {
		return tv.SwitchInput(r.ID)
	})))
	mux.HandleFunc("POST /api/tv/button", sv.auth(tvHandler(sv, func(tv *lg.TV, r struct {
		Name string `json:"name"`
	}) error {
		return tv.Button(r.Name)
	})))
	mux.HandleFunc("POST /api/tv/type", sv.auth(tvHandler(sv, func(tv *lg.TV, r struct {
		Text string `json:"text"`
	}) error {
		return tv.Type(r.Text)
	})))
	mux.HandleFunc("POST /api/tv/app", sv.auth(tvHandler(sv, func(tv *lg.TV, r struct {
		ID string `json:"id"`
	}) error {
		return tv.Launch(r.ID)
	})))

	fmt.Printf("canape is running for home %q. Open this link once on each of your devices (same Wi-Fi):\n\n  %s/?key=%s\n\n", h.Name, sv.baseURL, h.State.Dashboard.Key)
	fmt.Println("It logs the device in, so only open it on your own. Ctrl+C to stop.")
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
	svg, err := wifi.SVG(wifi.QRText(h.Wifi.SSID, h.Wifi.Password))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, svg)
}

func (sv *server) validKey(k string) bool {
	return subtle.ConstantTimeCompare([]byte(k), []byte(sv.here.State.Dashboard.Key)) == 1
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
	tv, err := sv.here.ConnectTV()
	if err != nil {
		// Off and unreachable look the same from here. The page offers power on.
		writeJSON(w, home.TVStatus{})
		return
	}
	defer tv.Close()

	st, err := home.Status(tv)
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
		tvHandler(sv, func(tv *lg.TV, _ struct{}) error { return tv.TurnOff() })(w, r)
		return
	}
	sv.mu.Lock()
	err := sv.here.WakeTV()
	sv.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// tvHandler decodes the JSON body into T, connects to the TV and runs fn. An
// empty or already-read body leaves T at its zero value.
func tvHandler[T any](sv *server, fn func(*lg.TV, T) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req T
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		sv.mu.Lock()
		defer sv.mu.Unlock()
		tv, err := sv.here.ConnectTV()
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		defer tv.Close()
		if err := fn(tv, req); err != nil {
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
