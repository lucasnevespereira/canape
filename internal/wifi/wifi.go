// Package wifi turns a network name and password into the QR code phones
// join with, and shows it on a page or a TV.
package wifi

import (
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/internal/qrcode"
)

// QRText builds the payload phone cameras understand as "join this
// network". WPA covers WPA2 and WPA3.
func QRText(ssid, password string) string {
	esc := strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, `:`, `\:`, `"`, `\"`)
	return "WIFI:T:WPA;S:" + esc.Replace(ssid) + ";P:" + esc.Replace(password) + ";;"
}

//go:embed page.html
var pageHTML string

var page = template.Must(template.New("wifi").Parse(pageHTML))

// Page writes the big-QR page shown on TVs and phones.
func Page(w io.Writer, ssid, password string) error {
	data := struct {
		SSID, Password string
		QR             template.HTML
	}{SSID: ssid, Password: password}
	if ssid != "" {
		svg, err := qrcode.SVG(QRText(ssid, password))
		if err != nil {
			return err
		}
		data.QR = template.HTML(svg) // generated here, no user markup
	}
	return page.Execute(w, data)
}

// ShowOnTV puts the page on a TV without a running server: a throwaway one
// on a random port answers the TV's single request, then stops. The QR is
// inline in the page, so it stays on screen afterwards. host is this
// machine's LAN address, and open asks the TV's browser to load a URL.
func ShowOnTV(ssid, password, host string, open func(url string) error) error {
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
		Page(w, ssid, password)
		select {
		case loaded <- struct{}{}:
		default:
		}
	})}
	go srv.Serve(ln)
	defer srv.Close()

	if err := open(fmt.Sprintf("http://%s:%d/wifi", host, ln.Addr().(*net.TCPAddr).Port)); err != nil {
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
