package tv

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"
)

// Wake sends a Wake-on-LAN magic packet: 6 bytes of 0xFF then the MAC 16
// times. A TV that's off can't be dialed, so this is how every brand turns
// on, provided its network-standby setting is enabled.
func Wake(mac, ip string) error {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return err
	}
	pkt := bytes.Repeat([]byte{0xff}, 6)
	for range 16 {
		pkt = append(pkt, hw...)
	}
	// ponytail: the subnet broadcast assumes a /24.
	targets := []string{"255.255.255.255:9", ip[:strings.LastIndex(ip, ".")] + ".255:9"}
	var errs []error
	for _, a := range targets {
		c, err := net.Dial("udp", a)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_, err = c.Write(pkt)
		c.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == len(targets) {
		return fmt.Errorf("send wake packet: %w", errors.Join(errs...))
	}
	return nil
}
