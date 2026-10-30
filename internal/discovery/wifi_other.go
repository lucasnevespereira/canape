//go:build !darwin

package discovery

// ponytail: macOS only. Linux has `iw dev <iface> link`, add it when someone
// runs the report there.
func WiFi() *Link { return nil }
