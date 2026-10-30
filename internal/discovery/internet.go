package discovery

import (
	"context"
	"fmt"
	"net"
	"time"
)

// Internet is the answer to "is it the internet or the Wi-Fi?".
type Internet struct {
	Box     bool          `json:"box"`     // the router answers
	Online  bool          `json:"online"`  // an address on the internet answers
	Names   bool          `json:"names"`   // name lookups (DNS) work
	Latency time.Duration `json:"latency"` // round trip to the internet
}

// CheckInternet takes up to a few seconds when something is down.
func CheckInternet() Internet {
	var in Internet
	if gw := gateway(); gw != "" {
		// Any answer, even "port closed", proves the box is there.
		c, err := net.DialTimeout("tcp", net.JoinHostPort(gw, "80"), 2*time.Second)
		in.Box = err == nil
		if c != nil {
			c.Close()
		}
	}
	// The fastest of three tries: a scan running at the same time can delay
	// one of them several times over.
	for i := 0; i < 3; i++ {
		start := time.Now()
		c, err := net.DialTimeout("tcp", "1.1.1.1:443", 3*time.Second)
		if err != nil {
			break
		}
		c.Close()
		if d := time.Since(start); !in.Online || d < in.Latency {
			in.Online, in.Latency = true, d
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := net.DefaultResolver.LookupHost(ctx, "www.apple.com")
	in.Names = err == nil
	return in
}

// Words is the plain answer, with what to do when something's wrong.
func (in Internet) Words() string {
	switch {
	case in.Online && in.Names:
		return "working, " + speedWords(in.Latency)
	case in.Online:
		return "connected, but websites won't open: name lookups (DNS) fail. Restarting the box usually fixes it"
	case in.Box:
		return "down. The Wi-Fi and the box work, the internet behind the box doesn't. Restart the box, or it's the provider"
	}
	return "down, and the box doesn't answer either. Check the Wi-Fi connection, then restart the box"
}

func speedWords(d time.Duration) string {
	ms := d.Milliseconds()
	words := "slow to respond"
	switch {
	case ms < 30:
		words = "quick to respond"
	case ms < 80:
		words = "normal response time"
	}
	return fmt.Sprintf("%s (%d ms)", words, ms)
}
