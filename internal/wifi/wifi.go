// Package wifi turns a network name and password into the QR code phones
// join with, drawn for a terminal, a web page or an image.
package wifi

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"rsc.io/qr"
)

// QRText builds the payload phone cameras understand as "join this
// network". WPA covers WPA2 and WPA3.
func QRText(ssid, password string) string {
	esc := strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, `:`, `\:`, `"`, `\"`)
	return "WIFI:T:WPA;S:" + esc.Replace(ssid) + ";P:" + esc.Replace(password) + ";;"
}

// quiet is the blank margin scanners need around the code, in modules.
const quiet = 4

// SVG draws the code as one SVG path, crisp at any size.
func SVG(text string) (string, error) {
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

// Terminal draws two QR rows per text line with half blocks. Colors are
// forced to white on black, so the code scans in light and dark terminals.
func Terminal(text string) (string, error) {
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

// PNG draws the code as a size×size image, for app icons.
func PNG(text string, size int) ([]byte, error) {
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
		svg, err := SVG(QRText(ssid, password))
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
