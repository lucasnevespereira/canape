package lg

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WebApp is a webOS web app built on the fly: one page and its icons.
type WebApp struct {
	ID, Title       string
	HTML            []byte
	Icon, LargeIcon []byte // PNG, 80 and 130 px
}

const appVersion = "1.0.0"

// WriteApp lays out a webOS web app in dir, ready for ares-package.
func WriteApp(dir string, a WebApp) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	appinfo, _ := json.MarshalIndent(map[string]any{
		"id":        a.ID,
		"version":   appVersion,
		"vendor":    "canape",
		"type":      "web",
		"main":      "index.html",
		"title":     a.Title,
		"icon":      "icon.png",
		"largeIcon": "largeIcon.png",
		// The page handles the Back key (461) itself and closes the app.
		"disableBackHistoryAPI": true,
	}, "", "  ")
	for name, b := range map[string][]byte{
		"appinfo.json":  appinfo,
		"index.html":    a.HTML,
		"icon.png":      a.Icon,
		"largeIcon.png": a.LargeIcon,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// aresDevice is the name canape registers the TV under in LG's CLI.
const aresDevice = "canape-tv"

// InstallApp packages an app and installs it on the TV with LG's ares CLI,
// then opens it. It talks to the user on the terminal: the first run asks
// for the passphrase shown in Developer Mode. Everything else ares needs is
// known: Developer Mode always listens on port 9922 as "prisoner".
func InstallApp(ip string, a WebApp) error {
	for _, bin := range []string{"ares-setup-device", "ares-novacom", "ares-package", "ares-install", "ares-launch"} {
		if _, err := exec.LookPath(bin); err != nil {
			return errors.New("LG's CLI isn't installed. Run: npm install -g @webos-tools/cli")
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
	appDir := filepath.Join(dir, "app")
	if err := WriteApp(appDir, a); err != nil {
		return err
	}
	if err := run("ares-package", appDir, "-o", dir); err != nil {
		return err
	}
	ipk := filepath.Join(dir, a.ID+"_"+appVersion+"_all.ipk")
	if err := run("ares-install", "--device", aresDevice, ipk); err != nil {
		return fmt.Errorf("install failed. Is Developer Mode on (Dev Mode Status)? %w", err)
	}
	return run("ares-launch", "--device", aresDevice, a.ID)
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
