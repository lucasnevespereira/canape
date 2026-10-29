package main

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
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/lucasnevespereira/canape/lgtv"
)

//go:embed web/index.html
var indexHTML []byte

type server struct {
	// ponytail: one lock for every TV request. It keeps one socket open at a
	// time and guards the state file, which connect may rewrite. Fine for a
	// household; split it if requests ever queue up.
	mu      sync.Mutex
	state   *state
	baseURL string // how the TV reaches this server
}

func serve(addr string) error {
	s, err := loadState()
	if err != nil {
		return err
	}
	if s.Dashboard.Key == "" {
		b := make([]byte, 16)
		rand.Read(b)
		s.Dashboard.Key = hex.EncodeToString(b)
		if err := saveState(s); err != nil {
			return err
		}
	}
	port := addr[strings.LastIndex(addr, ":")+1:]
	sv := &server{state: s, baseURL: "http://" + localIP() + ":" + port}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", sv.index)
	// The Wi-Fi page is public: the TV's browser has no login cookie, and
	// anyone who can reach this server is already on the Wi-Fi.
	mux.HandleFunc("GET /wifi", wifiHandler)
	mux.HandleFunc("GET /wifi.svg", wifiSVGHandler)
	mux.HandleFunc("GET /api/wifi", sv.auth(func(w http.ResponseWriter, r *http.Request) {
		s, err := loadState()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]string{"ssid": s.Wifi.SSID})
	}))
	mux.HandleFunc("POST /api/wifi/tv", sv.auth(tvHandler(sv, func(tv *lgtv.TV, _ struct{}) error {
		return tv.Open(sv.baseURL + "/wifi")
	})))
	mux.HandleFunc("GET /api/tv", sv.auth(sv.tvStatus))
	mux.HandleFunc("POST /api/tv/power", sv.auth(sv.tvPower))
	mux.HandleFunc("POST /api/tv/volume", sv.auth(tvHandler(sv, func(tv *lgtv.TV, r struct {
		Volume int `json:"volume"`
	}) error {
		return tv.SetVolume(r.Volume)
	})))
	mux.HandleFunc("POST /api/tv/mute", sv.auth(tvHandler(sv, func(tv *lgtv.TV, r struct {
		Mute bool `json:"mute"`
	}) error {
		return tv.SetMute(r.Mute)
	})))
	mux.HandleFunc("POST /api/tv/input", sv.auth(tvHandler(sv, func(tv *lgtv.TV, r struct {
		ID string `json:"id"`
	}) error {
		return tv.SwitchInput(r.ID)
	})))
	mux.HandleFunc("POST /api/tv/button", sv.auth(tvHandler(sv, func(tv *lgtv.TV, r struct {
		Name string `json:"name"`
	}) error {
		return tv.Button(r.Name)
	})))
	mux.HandleFunc("POST /api/tv/type", sv.auth(tvHandler(sv, func(tv *lgtv.TV, r struct {
		Text string `json:"text"`
	}) error {
		return tv.Type(r.Text)
	})))
	mux.HandleFunc("POST /api/tv/app", sv.auth(tvHandler(sv, func(tv *lgtv.TV, r struct {
		ID string `json:"id"`
	}) error {
		return tv.Launch(r.ID)
	})))

	fmt.Printf("canape is running. Open this link once on each of your devices (same Wi-Fi):\n\n  %s/?key=%s\n\n", sv.baseURL, s.Dashboard.Key)
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
func wifiHandler(w http.ResponseWriter, r *http.Request) {
	s, err := loadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := renderWifiPage(w, s.Wifi.SSID, s.Wifi.Password); err != nil {
		log.Print(err)
	}
}

func wifiSVGHandler(w http.ResponseWriter, r *http.Request) {
	s, err := loadState()
	if err != nil || s.Wifi.SSID == "" {
		http.NotFound(w, r)
		return
	}
	svg, err := qrSVG(wifiQRText(s.Wifi.SSID, s.Wifi.Password))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, svg)
}

func (sv *server) validKey(k string) bool {
	return subtle.ConstantTimeCompare([]byte(k), []byte(sv.state.Dashboard.Key)) == 1
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

type tvStatus struct {
	On      bool         `json:"on"`
	Volume  int          `json:"volume"`
	Muted   bool         `json:"muted"`
	Current string       `json:"current"` // app on screen; inputs count as apps
	Inputs  []lgtv.Input `json:"inputs"`
	Apps    []lgtv.App   `json:"apps"`
}

func (sv *server) tvStatus(w http.ResponseWriter, r *http.Request) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	tv, err := connect(sv.state)
	if err != nil {
		// Off and unreachable look the same from here. The page offers power on.
		writeJSON(w, tvStatus{})
		return
	}
	defer tv.Close()

	st, err := statusOf(tv)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, st)
}

// statusOf reads what the dashboard and the tv_status MCP tool show.
func statusOf(tv *lgtv.TV) (tvStatus, error) {
	st := tvStatus{On: true}
	v, err := tv.Volume()
	if err != nil {
		return st, err
	}
	st.Volume, st.Muted = v.Level, v.Muted
	st.Current, _ = tv.ForegroundApp()
	inputs, err := tv.Inputs()
	if err != nil {
		return st, err
	}
	for _, in := range inputs {
		if strings.HasPrefix(in.ID, "HDMI") { // AV and component inputs are legacy noise
			st.Inputs = append(st.Inputs, in)
		}
	}
	st.Apps, err = tv.Apps()
	return st, err
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
		tvHandler(sv, func(tv *lgtv.TV, _ struct{}) error { return tv.TurnOff() })(w, r)
		return
	}
	sv.mu.Lock()
	ip, mac := sv.state.TV.IP, sv.state.TV.MAC
	sv.mu.Unlock()
	if mac == "" {
		writeError(w, http.StatusConflict, errors.New("no MAC address saved for the TV. Run: canape tv pair"))
		return
	}
	if err := lgtv.Wake(mac, ip); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// tvHandler decodes the JSON body into T, connects to the TV and runs fn. An
// empty or already-read body leaves T at its zero value.
func tvHandler[T any](sv *server, fn func(*lgtv.TV, T) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req T
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		sv.mu.Lock()
		defer sv.mu.Unlock()
		tv, err := connect(sv.state)
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

// localIP returns this machine's LAN address. Dialing UDP sends nothing; it
// only picks the outgoing interface.
func localIP() string {
	c, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return "localhost"
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}
