package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/freebox"
	"github.com/lucasnevespereira/canape/lgtv"
	"rsc.io/qr"
)

//go:embed web/wifi.html
var wifiHTML string

var wifiPage = template.Must(template.New("wifi").Parse(wifiHTML))

func wifiCommand(args []string) error {
	s, err := loadState()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "set" {
		return wifiSet(s)
	}
	if s.Wifi.SSID == "" {
		return errors.New("no Wi-Fi saved yet. Run: canape wifi set")
	}
	if len(args) > 0 && args[0] == "tv" {
		if err := wifiOnTV(s); err != nil {
			return err
		}
		fmt.Println("The Wi-Fi QR is on the TV. Press Back on the remote to close it.")
		return nil
	}
	if len(args) > 0 && args[0] == "tvapp" {
		dir := "build/wifi-app"
		if len(args) > 1 {
			dir = args[1]
		}
		return wifiTVApp(s, dir)
	}
	out, err := qrTerminal(wifiQRText(s.Wifi.SSID, s.Wifi.Password))
	if err != nil {
		return err
	}
	fmt.Print(out)
	fmt.Printf("Network  %s\nPassword %s\n", s.Wifi.SSID, s.Wifi.Password)
	return nil
}

func wifiSet(s *state) error {
	ssid := s.Wifi.SSID
	if ssid == "" && s.Freebox.AppToken != "" {
		if fbx, err := freebox.New(s.Freebox.AppToken); err == nil {
			ssid, _ = fbx.SSID()
		}
	}
	in := bufio.NewReader(os.Stdin)
	s.Wifi.SSID = prompt(in, "Network name", ssid)
	fmt.Println("The password ends up in the QR code and on the /wifi page, so it isn't hidden here either.")
	s.Wifi.Password = prompt(in, "Password", "")
	if s.Wifi.SSID == "" || s.Wifi.Password == "" {
		return errors.New("network name and password are both needed")
	}
	if err := saveState(s); err != nil {
		return err
	}
	fmt.Println("Saved. Show it with: canape wifi")
	return nil
}

func prompt(in *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := in.ReadString('\n')
	// Only the newline: spaces can be part of a Wi-Fi password.
	if line = strings.TrimRight(line, "\r\n"); line == "" {
		return def
	}
	return line
}

// renderWifiPage writes the big-QR page served at /wifi and baked into the TV app.
func renderWifiPage(w io.Writer, ssid, password string) error {
	data := struct {
		SSID, Password string
		QR             template.HTML
	}{SSID: ssid, Password: password}
	if ssid != "" {
		svg, err := qrSVG(wifiQRText(ssid, password))
		if err != nil {
			return err
		}
		data.QR = template.HTML(svg) // generated here, no user markup
	}
	return wifiPage.Execute(w, data)
}

// wifiTVApp writes a webOS app that shows the Wi-Fi QR. The QR is baked in,
// so it works with canape stopped; rebuild it after changing the password.
// `canape tv install-app` packages and installs it with LG's CLI.
func wifiTVApp(s *state, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var page bytes.Buffer
	if err := renderWifiPage(&page, s.Wifi.SSID, s.Wifi.Password); err != nil {
		return err
	}
	text := wifiQRText(s.Wifi.SSID, s.Wifi.Password)
	icon, err := qrPNG(text, 80)
	if err != nil {
		return err
	}
	largeIcon, err := qrPNG(text, 130)
	if err != nil {
		return err
	}
	appinfo, _ := json.MarshalIndent(map[string]any{
		"id":        "com.canape.wifi",
		"version":   "1.0.0",
		"vendor":    "canape",
		"type":      "web",
		"main":      "index.html",
		"title":     "Wi-Fi",
		"icon":      "icon.png",
		"largeIcon": "largeIcon.png",
		// The page handles the Back key itself and closes the app.
		"disableBackHistoryAPI": true,
	}, "", "  ")
	for name, b := range map[string][]byte{
		"appinfo.json":  appinfo,
		"index.html":    page.Bytes(),
		"icon.png":      icon,
		"largeIcon.png": largeIcon,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			return err
		}
	}
	fmt.Println("Wrote the TV app to", dir)
	return nil
}

// wifiOnTV opens the Wi-Fi page in the TV's browser without `canape serve`:
// a throwaway server answers the TV's one request, then stops. The QR is
// inline in the page, so it stays on screen afterwards.
func wifiOnTV(s *state) error {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	loaded := make(chan struct{}, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wifi" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderWifiPage(w, s.Wifi.SSID, s.Wifi.Password)
		select {
		case loaded <- struct{}{}:
		default:
		}
	})}
	go srv.Serve(ln)
	defer srv.Close()

	tv, err := connect(s)
	if err != nil {
		return err
	}
	defer tv.Close()
	url := fmt.Sprintf("http://%s:%d/wifi", localIP(), ln.Addr().(*net.TCPAddr).Port)
	if err := tv.Open(url); err != nil {
		return err
	}
	select {
	case <-loaded:
		time.Sleep(time.Second) // let the response reach the TV before closing
		return nil
	case <-time.After(30 * time.Second):
		return errors.New("the TV never loaded the page. Is a firewall on this machine blocking incoming connections?")
	}
}

// aresDevice is the name canape registers the TV under in LG's CLI.
const aresDevice = "canape-tv"

// tvInstallApp builds the Wi-Fi app and installs it with LG's ares CLI.
// Everything ares needs is known except the passphrase the TV shows: the IP
// comes from pairing, and Developer Mode always listens on port 9922 as
// "prisoner".
func tvInstallApp(s *state) error {
	if s.Wifi.SSID == "" {
		return errors.New("no Wi-Fi saved yet. Run: canape wifi set")
	}
	for _, bin := range []string{"ares-setup-device", "ares-novacom", "ares-package", "ares-install", "ares-launch"} {
		if _, err := exec.LookPath(bin); err != nil {
			return errors.New("LG's CLI isn't installed. Run: npm install -g @webos-tools/cli")
		}
	}
	ip := s.TV.IP
	if ip == "" {
		var err error
		if ip, err = lgtv.Discover(3 * time.Second); err != nil {
			return err
		}
	}

	// Modify keeps the key ares saved earlier; add only the first time.
	list, _ := exec.Command("ares-setup-device", "--list").Output()
	mode := "-a"
	if strings.Contains(string(list), aresDevice) {
		mode = "-m"
	}
	info := fmt.Sprintf("{'host': '%s', 'port': '9922', 'username': 'prisoner'}", ip)
	if err := run("ares-setup-device", mode, aresDevice, "-i", info); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(home, ".ssh", aresDevice+"_webos")); err != nil {
		fmt.Println("\nFirst install: canape needs the TV's Developer Mode key.")
		// The key server listens on 9991 while "Key Server" is on, and turns
		// itself off after a while.
		if !waitPort(ip, "9991", 2*time.Minute, func() {
			fmt.Println(`On the TV, open Developer Mode and turn on "Key Server". Waiting for it...`)
		}) {
			return errors.New(`the TV's Key Server never came on. Turn it on in Developer Mode and run this again`)
		}
		fmt.Println("Key Server found. Type the passphrase shown on the TV.")
		if err := run("ares-novacom", "--device", aresDevice, "--getkey"); err != nil {
			return fmt.Errorf("getting the key failed. Is Key Server on? %w", err)
		}
	}

	dir, err := os.MkdirTemp("", "canape-tvapp")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	app := filepath.Join(dir, "app")
	if err := wifiTVApp(s, app); err != nil {
		return err
	}
	if err := run("ares-package", app, "-o", dir); err != nil {
		return err
	}
	if err := run("ares-install", "--device", aresDevice, filepath.Join(dir, "com.canape.wifi_1.0.0_all.ipk")); err != nil {
		return fmt.Errorf("install failed. Is Developer Mode on (Dev Mode Status)? %w", err)
	}
	if err := run("ares-launch", "--device", aresDevice, "com.canape.wifi"); err != nil {
		return err
	}
	fmt.Println("\nInstalled. The Wi-Fi app is in the TV's app bar. Run this again after changing the Wi-Fi password.")
	return nil
}

// waitPort polls until host:port accepts connections or timeout passes.
// waiting is called once, if the port isn't open on the first try.
func waitPort(host, port string, timeout time.Duration, waiting func()) bool {
	deadline := time.Now().Add(timeout)
	for first := true; time.Now().Before(deadline); first = false {
		if c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), time.Second); err == nil {
			c.Close()
			return true
		}
		if first {
			waiting()
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// run streams a command's output, and its prompts, to the terminal.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// qrPNG draws the code as a size×size image, for the TV app icons.
func qrPNG(text string, size int) ([]byte, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	n := c.Size + 2*quiet
	img := image.NewGray(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !c.Black(x*n/size-quiet, y*n/size-quiet) {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	var buf bytes.Buffer
	err = png.Encode(&buf, img)
	return buf.Bytes(), err
}

// wifiQRText builds the payload phone cameras understand as "join this
// network". WPA covers WPA2 and WPA3.
func wifiQRText(ssid, password string) string {
	esc := strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, `:`, `\:`, `"`, `\"`)
	return "WIFI:T:WPA;S:" + esc.Replace(ssid) + ";P:" + esc.Replace(password) + ";;"
}

// quiet is the blank margin scanners need around the code, in modules.
const quiet = 4

// qrSVG draws the code as one SVG path, crisp at any size.
func qrSVG(text string) (string, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	n := c.Size + 2*quiet
	var path strings.Builder
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if c.Black(x, y) {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+
		`<rect width="%d" height="%d" fill="#fff"/><path d="%s" fill="#000"/></svg>`, n, n, n, n, path.String()), nil
}

// qrTerminal draws two QR rows per text line with half blocks. Colors are
// forced to white on black, so the code scans in light and dark terminals.
func qrTerminal(text string) (string, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	// Black returns false outside the code, which draws the quiet zone.
	dark := func(x, y int) bool { return c.Black(x-quiet, y-quiet) }
	n := c.Size + 2*quiet
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		b.WriteString("\x1b[97;40m")
		for x := 0; x < n; x++ {
			switch top, bottom := dark(x, y), dark(x, y+1); {
			case !top && !bottom:
				b.WriteString("█")
			case !top:
				b.WriteString("▀")
			case !bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String(), nil
}
