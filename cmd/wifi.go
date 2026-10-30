package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/qrcode"
	"github.com/lucasnevespereira/canape/internal/router/freebox"
	"github.com/lucasnevespereira/canape/internal/wifi"
)

func wifiCommand(args []string) error {
	s, err := home.Load()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "set" {
		if !askWifi(s, bufio.NewReader(os.Stdin), "") {
			return errors.New("network name and password are both needed")
		}
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Println("Saved. Show it with: canape wifi tv")
		return nil
	}
	if s.Wifi.SSID == "" {
		return errors.New("no Wi-Fi saved for this network. Run: canape setup")
	}
	if len(args) > 0 && args[0] == "tv" {
		t, err := s.ConnectTV()
		if err != nil {
			return err
		}
		defer t.Close()
		if err := wifi.ShowOnTV(s.Wifi.SSID, s.Wifi.Password, discovery.LocalIP(), t.Open); err != nil {
			return err
		}
		fmt.Println("The Wi-Fi QR is on the TV. Press Back on the remote to close it.")
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("unknown command %q. Run: canape help", "wifi "+args[0])
	}
	out, err := qrcode.Terminal(wifi.QRText(s.Wifi.SSID, s.Wifi.Password))
	if err != nil {
		return err
	}
	fmt.Print(out)
	fmt.Printf("Network  %s\nPassword %s\n", s.Wifi.SSID, s.Wifi.Password)
	return nil
}

// askWifi asks for the network name, prefilled from the Freebox when there
// is one, and the password. It reports whether both were given. Saving is
// up to the caller.
func askWifi(s *home.Here, in *bufio.Reader, indent string) bool {
	ssid := s.Wifi.SSID
	if ssid == "" && s.Freebox.AppToken != "" {
		if fbx, err := freebox.New(s.Freebox.AppToken); err == nil {
			ssid, _ = fbx.SSID()
		}
	}
	name := prompt(in, indent+"Network name", ssid)
	fmt.Println(indent + "The password ends up in the QR code, so it isn't hidden here either. Leave it empty to skip.")
	password := prompt(in, indent+"Password", "")
	if name == "" || password == "" {
		return false
	}
	s.Wifi.SSID, s.Wifi.Password = name, password
	return true
}
