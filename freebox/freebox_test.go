package freebox

import (
	"encoding/json"
	"testing"
)

func TestIPv4PrefersActiveAddress(t *testing.T) {
	var h Host
	json.Unmarshal([]byte(`{"l3connectivities":[
		{"addr":"192.168.1.50","af":"ipv4","active":false},
		{"addr":"fe80::1","af":"ipv6","active":true},
		{"addr":"192.168.1.23","af":"ipv4","active":true}]}`), &h)
	if got := h.IPv4(); got != "192.168.1.23" {
		t.Errorf("IPv4() = %q", got)
	}
}
