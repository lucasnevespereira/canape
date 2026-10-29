// Command scan lists the devices on the local network and prints what canape
// needs to know about them: IPs, MACs, models and which control ports answer.
//
//	go run ./cmd/scan
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type device struct {
	ip       string
	mac      string
	host     string // reverse DNS
	name     string // SSDP friendly name
	alias    string // mDNS instance name
	model    string // SSDP manufacturer + model
	hw       string // mDNS model, e.g. AppleTV11,1
	server   string // SSDP Server header, often carries the OS version
	services map[string]bool
	open     []int
}

var (
	mu      sync.Mutex
	devices = map[string]*device{}
	localIP string
	// ponytail: assumes a /24, which is what home routers hand out. Read the netmask if that ever breaks.
	subnet string
)

// get must be called with mu held.
func get(ip string) *device {
	d := devices[ip]
	if d == nil {
		d = &device{ip: ip, services: map[string]bool{}}
		devices[ip] = d
	}
	return d
}

func main() {
	c, err := net.Dial("udp", "192.0.2.1:9") // UDP dial sends nothing, it only picks the outgoing interface
	if err != nil {
		fmt.Fprintln(os.Stderr, "no network:", err)
		os.Exit(1)
	}
	localIP = c.LocalAddr().(*net.UDPAddr).IP.String()
	c.Close()
	subnet = localIP[:strings.LastIndex(localIP, ".")+1]

	fmt.Println("canape scan (takes a few seconds)")
	fmt.Printf("\nThis machine   %s\n", localIP)
	freebox()

	var wg sync.WaitGroup
	for _, f := range []func(){sweep, ssdp, mdns} {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	wg.Wait()
	arp()
	enrich()
	report()
}

func freebox() {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://mafreebox.freebox.fr/api_version")
	if err != nil {
		fmt.Println("Freebox        not found")
		return
	}
	defer resp.Body.Close()
	var v struct {
		Model   string `json:"box_model_name"`
		Base    string `json:"api_base_url"`
		Version string `json:"api_version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		fmt.Println("Freebox        unexpected api_version reply:", err)
		return
	}
	nick := ""
	for gen, n := range map[string]string{"v6": "Revolution", "v7": "Delta", "v8": "Pop"} {
		if strings.Contains(v.Model, "Freebox "+gen) {
			nick = " (" + n + ")"
		}
	}
	major, _, _ := strings.Cut(v.Version, ".")
	fmt.Printf("Freebox        %s%s, API %s\n", v.Model, nick, v.Version)
	fmt.Printf("               http://mafreebox.freebox.fr%sv%s/\n", v.Base, major)
}

// sweep sends one byte to every address in the subnet. The OS has to resolve
// each MAC to send it, so devices that never advertise anything still end up
// in the ARP cache.
func sweep() {
	for i := 1; i < 255; i++ {
		if c, err := net.Dial("udp", fmt.Sprintf("%s%d:9", subnet, i)); err == nil {
			c.Write([]byte{0})
			c.Close()
		}
	}
	time.Sleep(2 * time.Second)
}

func ssdp() {
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
		mu.Lock()
		d := get(ip)
		st := shortST(resp.Header.Get("ST"))
		if st != "" {
			d.services["ssdp:"+st] = true
		}
		if s := resp.Header.Get("Server"); s != "" {
			d.server = s
		}
		mu.Unlock()
		// LG serves several descriptions; only the second-screen one has the real model number.
		if _, ok := locations[ip]; !ok || st == "webos-second-screen" {
			locations[ip] = resp.Header.Get("Location")
		}
	}

	var wg sync.WaitGroup
	for ip, loc := range locations {
		wg.Add(1)
		go func() { defer wg.Done(); describe(ip, loc) }()
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
func describe(ip, loc string) {
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
	mu.Lock()
	defer mu.Unlock()
	d := get(ip)
	d.name = dd.FriendlyName
	d.model = strings.TrimSpace(dd.Manufacturer + " " + model)
}

func mdns() {
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

	mu.Lock()
	defer mu.Unlock()
	for inst, typ := range instances {
		// Prefer the SRV/A records: a sleep proxy can answer for another device.
		ip := addrs[targets[inst]]
		if ip == "" {
			ip = from[inst]
		}
		if !strings.HasPrefix(ip, subnet) {
			continue
		}
		d := get(ip)
		short := strings.TrimPrefix(strings.Split(typ, ".")[0], "_")
		d.services["mdns:"+short] = true
		if d.alias == "" || short == "airplay" || short == "googlecast" {
			d.alias = strings.TrimSuffix(inst, "."+typ)
		}
		// Each service names its model key differently, and raop's "md" means metadata, not model.
		key := map[string]string{"airplay": "model", "companion-link": "rpMd", "googlecast": "md"}[short]
		for _, kv := range txt[inst] {
			if k, v, _ := strings.Cut(kv, "="); key != "" && k == key && d.hw == "" {
				d.hw = v
			}
		}
	}
}

var arpLine = regexp.MustCompile(`\((\d+\.\d+\.\d+\.\d+)\) at ([0-9a-fA-F:]+)`)

func arp() {
	out, _ := exec.Command("arp", "-an").Output()
	for _, m := range arpLine.FindAllStringSubmatch(string(out), -1) {
		ip, mac := m[1], padMAC(m[2])
		if !strings.HasPrefix(ip, subnet) || strings.HasSuffix(ip, ".255") || mac == "ff:ff:ff:ff:ff:ff" {
			continue
		}
		get(ip).mac = mac
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.String() == localIP {
				get(localIP).mac = iface.HardwareAddr.String()
			}
		}
	}
}

// padMAC fixes macOS printing "aa:bb:cc:d:ee:ff" for "aa:bb:cc:0d:ee:ff".
func padMAC(s string) string {
	parts := strings.Split(strings.ToLower(s), ":")
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

func enrich() {
	var wg sync.WaitGroup
	for _, d := range devices {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if names, err := net.DefaultResolver.LookupAddr(ctx, d.ip); err == nil && len(names) > 0 {
				mu.Lock()
				d.host = strings.TrimSuffix(names[0], ".")
				mu.Unlock()
			}
			for p := range ports {
				if c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", d.ip, p), 700*time.Millisecond); err == nil {
					c.Close()
					mu.Lock()
					d.open = append(d.open, p)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
}

func (d *device) has(svc string) bool { return d.services[svc] }

func kind(d *device) string {
	switch {
	case d.ip == localIP:
		return "this machine"
	case d.has("ssdp:webos-second-screen"), strings.HasPrefix(d.host, "lgwebostv"):
		return "LG webOS TV"
	case d.has("mdns:fbx-api"):
		return "Freebox"
	case strings.HasPrefix(d.hw, "AppleTV"):
		return "Apple TV"
	case d.has("mdns:googlecast"), d.has("mdns:androidtvremote2"), contains(d.open, 5555):
		return "Android TV / Google Cast"
	case strings.HasPrefix(d.host, "freebox-player"):
		return "Freebox Player"
	case d.has("mdns:airplay"):
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

func report() {
	var known, other []*device
	for _, d := range devices {
		if kind(d) != "" || d.name != "" || d.alias != "" || len(d.services) > 0 || len(d.open) > 0 {
			known = append(known, d)
		} else {
			other = append(other, d)
		}
	}
	byIP := func(s []*device) {
		sort.Slice(s, func(i, j int) bool { return ipNum(s[i].ip) < ipNum(s[j].ip) })
	}
	byIP(known)
	byIP(other)

	fmt.Printf("\nDevices that advertise themselves (%d)\n", len(known))
	for _, d := range known {
		title := firstNonEmpty(d.name, d.alias, d.host, "unnamed")
		if k := kind(d); k != "" {
			title += "  [" + k + "]"
		}
		fmt.Printf("\n  %-15s %s\n", d.ip, title)
		line("mac", d.mac)
		line("host", d.host)
		model := d.model
		if !strings.Contains(model, d.hw) {
			model += " " + d.hw
		}
		line("model", strings.TrimSpace(model))
		line("software", d.server)
		var svcs []string
		for s := range d.services {
			svcs = append(svcs, s)
		}
		sort.Strings(svcs)
		line("services", strings.Join(svcs, " "))
		sort.Ints(d.open)
		for _, p := range d.open {
			label := ""
			if ports[p].kind == kind(d) {
				label = ports[p].label
			}
			line("open", strings.TrimSpace(fmt.Sprintf("%d  %s", p, label)))
		}
		if kind(d) == "LG webOS TV" {
			line("canape", "TV IP and MAC above are what canape needs (MAC is for Wake-on-LAN)")
		}
	}

	if len(other) > 0 {
		fmt.Printf("\nOther devices on the network (%d)\n\n", len(other))
		for _, d := range other {
			fmt.Printf("  %-15s %-17s  %s\n", d.ip, d.mac, d.host)
		}
	}
	fmt.Println()
}

func line(label, value string) {
	if value != "" {
		fmt.Printf("                  %-9s %s\n", label, value)
	}
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func ipNum(ip string) uint32 {
	v4 := net.ParseIP(ip).To4()
	if v4 == nil {
		return 0
	}
	return uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
}
