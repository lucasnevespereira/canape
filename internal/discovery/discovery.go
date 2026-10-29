// Package discovery finds what's on the local network: SSDP and mDNS
// announcements, the ARP table, reverse DNS names and a few control ports.
package discovery

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type Device struct {
	IP, MAC  string
	Host     string // reverse DNS
	Name     string // SSDP friendly name
	Alias    string // mDNS instance name
	Model    string // SSDP manufacturer + model
	HW       string // mDNS model, e.g. AppleTV11,1
	Server   string // SSDP Server header, often carries the OS version
	Self     bool   // the machine running the scan
	Services map[string]bool
	Open     []int // control ports that answered, see PortLabel
}

// Freebox is what the box says about itself without authentication.
type Freebox struct {
	Model   string // e.g. "Freebox v8 (r1)"
	Nick    string // e.g. "Pop", empty when unknown
	Version string // API version, e.g. "16.1"
	BaseURL string // e.g. http://mafreebox.freebox.fr/api/v16/
}

type Result struct {
	LocalIP string
	Freebox *Freebox  // nil when there's no Freebox
	Devices []*Device // sorted by IP
}

// LocalIP returns this machine's LAN address. Dialing UDP sends nothing; it
// only picks the outgoing interface.
func LocalIP() string {
	c, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return "localhost"
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

type scanner struct {
	mu      sync.Mutex
	devices map[string]*Device
	localIP string
	// ponytail: assumes a /24, which is what home routers hand out. Read the netmask if that ever breaks.
	subnet string
}

// Scan takes a few seconds: it waits for announcements to come back.
func Scan() (*Result, error) {
	local := LocalIP()
	if local == "localhost" {
		return nil, fmt.Errorf("no network connection")
	}
	s := &scanner{devices: map[string]*Device{}, localIP: local, subnet: local[:strings.LastIndex(local, ".")+1]}
	res := &Result{LocalIP: local, Freebox: probeFreebox()}

	var wg sync.WaitGroup
	for _, f := range []func(){s.sweep, s.ssdp, s.mdns} {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	wg.Wait()
	s.arp()
	s.enrich()

	for _, d := range s.devices {
		res.Devices = append(res.Devices, d)
	}
	sort.Slice(res.Devices, func(i, j int) bool { return ipNum(res.Devices[i].IP) < ipNum(res.Devices[j].IP) })
	return res, nil
}

// get must be called with mu held.
func (s *scanner) get(ip string) *Device {
	d := s.devices[ip]
	if d == nil {
		d = &Device{IP: ip, Self: ip == s.localIP, Services: map[string]bool{}}
		s.devices[ip] = d
	}
	return d
}

func probeFreebox() *Freebox {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://mafreebox.freebox.fr/api_version")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var v struct {
		Model   string `json:"box_model_name"`
		Base    string `json:"api_base_url"`
		Version string `json:"api_version"`
	}
	if json.NewDecoder(resp.Body).Decode(&v) != nil || v.Version == "" {
		return nil
	}
	f := &Freebox{Model: v.Model, Version: v.Version}
	for gen, n := range map[string]string{"v6": "Revolution", "v7": "Delta", "v8": "Pop"} {
		if strings.Contains(v.Model, "Freebox "+gen) {
			f.Nick = n
		}
	}
	major, _, _ := strings.Cut(v.Version, ".")
	f.BaseURL = fmt.Sprintf("http://mafreebox.freebox.fr%sv%s/", v.Base, major)
	return f
}

// sweep sends one byte to every address in the subnet. The OS has to resolve
// each MAC to send it, so devices that never advertise anything still end up
// in the ARP cache.
func (s *scanner) sweep() {
	for i := 1; i < 255; i++ {
		if c, err := net.Dial("udp", fmt.Sprintf("%s%d:9", s.subnet, i)); err == nil {
			c.Write([]byte{0})
			c.Close()
		}
	}
	time.Sleep(2 * time.Second)
}

func (s *scanner) ssdp() {
	c, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return
	}
	defer c.Close()
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	// UDP drops packets, so each search goes out three times. LG answers its
	// own search type more reliably than ssdp:all.
	for range 3 {
		for _, st := range []string{"ssdp:all", "urn:lge-com:service:webos-second-screen:1"} {
			msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: " + st + "\r\n\r\n"
			c.WriteToUDP([]byte(msg), dst)
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.SetReadDeadline(time.Now().Add(3 * time.Second))

	locations := map[string]string{}
	buf := make([]byte, 8192)
	for {
		n, addr, err := c.ReadFromUDP(buf)
		if err != nil {
			break
		}
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(buf[:n])), nil)
		if err != nil {
			continue
		}
		ip := addr.IP.String()
		s.mu.Lock()
		d := s.get(ip)
		st := shortST(resp.Header.Get("ST"))
		if st != "" {
			d.Services["ssdp:"+st] = true
		}
		if v := resp.Header.Get("Server"); v != "" {
			d.Server = v
		}
		s.mu.Unlock()
		// LG serves several descriptions; only the second-screen one has the real model number.
		if _, ok := locations[ip]; !ok || st == "webos-second-screen" {
			locations[ip] = resp.Header.Get("Location")
		}
	}

	var wg sync.WaitGroup
	for ip, loc := range locations {
		wg.Add(1)
		go func() { defer wg.Done(); s.describe(ip, loc) }()
	}
	wg.Wait()
}

// shortST turns "urn:schemas-upnp-org:device:MediaRenderer:1" into "MediaRenderer"
// and drops the uuid and rootdevice entries every UPnP device repeats.
func shortST(st string) string {
	parts := strings.Split(st, ":")
	if parts[0] != "urn" || len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

// describe reads the UPnP device description an SSDP reply points to.
func (s *scanner) describe(ip, loc string) {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(loc)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var desc struct {
		Device struct {
			FriendlyName string `xml:"friendlyName"`
			Manufacturer string `xml:"manufacturer"`
			ModelName    string `xml:"modelName"`
			ModelNumber  string `xml:"modelNumber"`
		} `xml:"device"`
	}
	if xml.NewDecoder(resp.Body).Decode(&desc) != nil {
		return
	}
	dd := desc.Device
	model := dd.ModelNumber
	if model == "" {
		model = dd.ModelName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.get(ip)
	d.Name = dd.FriendlyName
	d.Model = strings.TrimSpace(dd.Manufacturer + " " + model)
}

func (s *scanner) mdns() {
	c, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return
	}
	defer c.Close()
	// Querying from a random port (not 5353) makes responders answer us
	// directly instead of multicasting (RFC 6762, legacy unicast).
	dst := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	ask := func(name string) {
		n, err := dnsmessage.NewName(name)
		if err != nil {
			return
		}
		b := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
		b.StartQuestions()
		b.Question(dnsmessage.Question{Name: n, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET})
		msg, _ := b.Finish()
		c.WriteToUDP(msg, dst)
	}
	const allTypes = "_services._dns-sd._udp.local."
	ask(allTypes)
	c.SetReadDeadline(time.Now().Add(3 * time.Second))

	asked := map[string]bool{}
	instances := map[string]string{} // instance -> service type
	from := map[string]string{}      // instance -> IP that answered
	targets := map[string]string{}   // instance -> host name (SRV)
	addrs := map[string]string{}     // host name -> IPv4 (A)
	txt := map[string][]string{}     // instance -> TXT key=value pairs
	buf := make([]byte, 9000)
	for {
		n, addr, err := c.ReadFromUDP(buf)
		if err != nil {
			break
		}
		var p dnsmessage.Parser
		if _, err := p.Start(buf[:n]); err != nil || p.SkipAllQuestions() != nil {
			continue
		}
		answers, _ := p.AllAnswers()
		p.SkipAllAuthorities()
		extras, _ := p.AllAdditionals()
		for _, r := range append(answers, extras...) {
			name := r.Header.Name.String()
			switch b := r.Body.(type) {
			case *dnsmessage.PTRResource:
				target := b.PTR.String()
				if name == allTypes {
					if !asked[target] {
						asked[target] = true
						ask(target)
					}
					continue
				}
				instances[target] = name
				from[target] = addr.IP.String()
			case *dnsmessage.SRVResource:
				targets[name] = b.Target.String()
			case *dnsmessage.AResource:
				addrs[name] = net.IP(b.A[:]).String()
			case *dnsmessage.TXTResource:
				txt[name] = b.TXT
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for inst, typ := range instances {
		// Prefer the SRV/A records: a sleep proxy can answer for another device.
		ip := addrs[targets[inst]]
		if ip == "" {
			ip = from[inst]
		}
		if !strings.HasPrefix(ip, s.subnet) {
			continue
		}
		d := s.get(ip)
		short := strings.TrimPrefix(strings.Split(typ, ".")[0], "_")
		d.Services["mdns:"+short] = true
		if d.Alias == "" || short == "airplay" || short == "googlecast" {
			d.Alias = strings.TrimSuffix(inst, "."+typ)
		}
		// Each service names its model key differently, and raop's "md" means metadata, not model.
		key := map[string]string{"airplay": "model", "companion-link": "rpMd", "googlecast": "md"}[short]
		for _, kv := range txt[inst] {
			if k, v, _ := strings.Cut(kv, "="); key != "" && k == key && d.HW == "" {
				d.HW = v
			}
		}
	}
}

var arpLine = regexp.MustCompile(`\((\d+\.\d+\.\d+\.\d+)\) at ([0-9a-fA-F:]+)`)

func (s *scanner) arp() {
	out, _ := exec.Command("arp", "-an").Output()
	for _, m := range arpLine.FindAllStringSubmatch(string(out), -1) {
		ip, mac := m[1], padMAC(m[2])
		if !strings.HasPrefix(ip, s.subnet) || strings.HasSuffix(ip, ".255") || mac == "ff:ff:ff:ff:ff:ff" {
			continue
		}
		s.get(ip).MAC = mac
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.String() == s.localIP {
				s.get(s.localIP).MAC = iface.HardwareAddr.String()
			}
		}
	}
}

// padMAC fixes macOS printing "aa:bb:cc:d:ee:ff" for "aa:bb:cc:0d:ee:ff".
func padMAC(m string) string {
	parts := strings.Split(strings.ToLower(m), ":")
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}

// Ports that tell us how a device can be controlled. The meaning only holds
// on the matching kind of device: a laptop with 3000 open is running a dev server.
var ports = map[int]struct{ kind, label string }{
	3000: {"LG webOS TV", "LG control (ws)"},
	3001: {"LG webOS TV", "LG control (wss)"},
	5555: {"Android TV / Google Cast", "ADB, Android network debugging"},
	8009: {"Android TV / Google Cast", "Google Cast"},
}

func (s *scanner) enrich() {
	var wg sync.WaitGroup
	for _, d := range s.devices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if names, err := net.DefaultResolver.LookupAddr(ctx, d.IP); err == nil && len(names) > 0 {
				s.mu.Lock()
				d.Host = strings.TrimSuffix(names[0], ".")
				s.mu.Unlock()
			}
			for p := range ports {
				if c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", d.IP, p), 700*time.Millisecond); err == nil {
					c.Close()
					s.mu.Lock()
					d.Open = append(d.Open, p)
					s.mu.Unlock()
				}
			}
			s.mu.Lock()
			sort.Ints(d.Open)
			s.mu.Unlock()
		}()
	}
	wg.Wait()
}

// PortLabel says what an open port is for, only when it fits the device.
func (d *Device) PortLabel(port int) string {
	if p, ok := ports[port]; ok && p.kind == d.Kind() {
		return p.label
	}
	return ""
}

// Kind guesses what the device is, or returns "" when it can't tell.
func (d *Device) Kind() string {
	has := func(svc string) bool { return d.Services[svc] }
	switch {
	case d.Self:
		return "this machine"
	case has("ssdp:webos-second-screen"), strings.HasPrefix(d.Host, "lgwebostv"):
		return "LG webOS TV"
	case has("mdns:fbx-api"):
		return "Freebox"
	case strings.HasPrefix(d.HW, "AppleTV"):
		return "Apple TV"
	case has("mdns:googlecast"), has("mdns:androidtvremote2"), contains(d.Open, 5555):
		return "Android TV / Google Cast"
	case strings.HasPrefix(d.Host, "freebox-player"):
		return "Freebox Player"
	case has("mdns:airplay"):
		return "AirPlay device"
	}
	return ""
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func ipNum(ip string) uint32 {
	v4 := net.ParseIP(ip).To4()
	if v4 == nil {
		return 0
	}
	return uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
}
