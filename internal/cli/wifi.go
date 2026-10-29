package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"

	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/router/freebox"
	"github.com/lucasnevespereira/canape/internal/store"
	"github.com/lucasnevespereira/canape/internal/wifi"
)

func wifiCommand(args []string) error {
	s, err := store.Load()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "set" {
		return wifiSet(s)
	}
	if s.Wifi.SSID == "" {
		return errors.New("no Wi-Fi saved yet. Run: canape wifi set")
	}
	if len(args) > 0 && args[0] == "tv" {
		t, err := home.ConnectTV(s)
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
	out, err := wifi.Terminal(wifi.QRText(s.Wifi.SSID, s.Wifi.Password))
	if err != nil {
		return err
	}
	fmt.Print(out)
	fmt.Printf("Network  %s\nPassword %s\n", s.Wifi.SSID, s.Wifi.Password)
	return nil
}

func wifiSet(s *store.State) error {
	ssid := s.Wifi.SSID
	if ssid == "" && s.Freebox.AppToken != "" {
		if fbx, err := freebox.New(s.Freebox.AppToken); err == nil {
			ssid, _ = fbx.SSID()
		}
	}
	in := bufio.NewReader(os.Stdin)
	s.Wifi.SSID = prompt(in, "Network name", ssid)
	fmt.Println("The password ends up in the QR code and on the /wifi page, so it isn't hidden here either.")
	s.Wifi.Password = prompt(in, "Password", "")
	if s.Wifi.SSID == "" || s.Wifi.Password == "" {
		return errors.New("network name and password are both needed")
	}
	if err := s.Save(); err != nil {
		return err
	}
	fmt.Println("Saved. Show it with: canape wifi")
	return nil
}
