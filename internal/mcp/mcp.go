// Package mcp exposes canape to MCP clients like Claude, over stdio.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/internal/computer"
	"github.com/lucasnevespereira/canape/internal/discovery"
	"github.com/lucasnevespereira/canape/internal/home"
	"github.com/lucasnevespereira/canape/internal/tv"
	"github.com/lucasnevespereira/canape/internal/version"
	"github.com/lucasnevespereira/canape/internal/wifi"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// reply is what action tools return: one sentence the model can relay.
type reply struct {
	Message string `json:"message"`
}

type nameInput struct {
	Name string `json:"name" jsonschema:"name as listed by tv_status. Loose matching works, e.g. hdmi1, apple, youtube"`
}

// Run serves the CLI's functions as MCP tools over stdio.
// Each call reads the state file and connects fresh, like a CLI command, so
// nothing has to keep running between calls.
func Run() error {
	s := sdk.NewServer(&sdk.Implementation{Name: "canape", Version: version.Get()}, nil)
	readOnly := &sdk.ToolAnnotations{ReadOnlyHint: true}

	sdk.AddTool(s, &sdk.Tool{
		Name:        "tv_status",
		Description: "Whether the TV is on, its volume, what's on screen, and its inputs and apps. Call it to learn input and app names.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, home.TVStatus, error) {
		s, err := home.Load()
		if err != nil {
			return nil, home.TVStatus{}, err
		}
		t, err := s.ConnectTV()
		if err != nil {
			return nil, home.TVStatus{}, nil // off or unreachable: on=false
		}
		defer t.Close()
		st, err := home.Status(t)
		return nil, st, err
	})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_power", Description: "Turn the TV on or off."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
			On bool `json:"on" jsonschema:"true to turn on, false to turn off"`
		}) (*sdk.CallToolResult, reply, error) {
			if in.On {
				s, err := home.Load()
				if err != nil {
					return nil, reply{}, err
				}
				return nil, reply{"Wake-on-LAN sent. The TV takes a few seconds to start."}, s.WakeTV()
			}
			return withTV(func(t tv.TV) (string, error) { return "TV off.", t.TurnOff() })
		})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_volume", Description: "Set the TV volume."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
			Level int `json:"level" jsonschema:"volume from 0 to 100"`
		}) (*sdk.CallToolResult, reply, error) {
			return withTV(func(t tv.TV) (string, error) {
				return fmt.Sprintf("Volume set to %d.", in.Level), t.SetVolume(in.Level)
			})
		})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_mute", Description: "Mute or unmute the TV."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
			Mute bool `json:"mute" jsonschema:"true to mute, false to unmute"`
		}) (*sdk.CallToolResult, reply, error) {
			return withTV(func(t tv.TV) (string, error) { return "Done.", t.SetMute(in.Mute) })
		})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_input", Description: "Switch the TV to an HDMI input, e.g. the Apple TV or a console."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in nameInput) (*sdk.CallToolResult, reply, error) {
			return withTV(func(t tv.TV) (string, error) {
				inputs, err := t.Inputs()
				if err != nil {
					return "", err
				}
				var choices []tv.Choice
				for _, i := range inputs {
					choices = append(choices, tv.Choice{ID: i.ID, Label: i.Label})
				}
				c, err := tv.Match(in.Name, choices)
				if err != nil {
					return "", err
				}
				return "Switched to " + c.Label + ".", t.SwitchInput(c.ID)
			})
		})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_launch_app", Description: "Open an app on the TV, e.g. YouTube or Prime Video."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in nameInput) (*sdk.CallToolResult, reply, error) {
			return withTV(func(t tv.TV) (string, error) {
				apps, err := t.Apps()
				if err != nil {
					return "", err
				}
				var choices []tv.Choice
				for _, a := range apps {
					choices = append(choices, tv.Choice{ID: a.ID, Label: a.Title})
				}
				c, err := tv.Match(in.Name, choices)
				if err != nil {
					return "", err
				}
				return "Opened " + c.Label + ".", t.Launch(c.ID)
			})
		})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_open_url", Description: "Open a web page in the TV's browser."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
			URL string `json:"url" jsonschema:"http or https URL"`
		}) (*sdk.CallToolResult, reply, error) {
			return withTV(func(t tv.TV) (string, error) { return "Opened " + in.URL + ".", t.Open(in.URL) })
		})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_press_key", Description: "Press a remote key on the TV."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
			Key string `json:"key" jsonschema:"one of HOME, BACK, UP, DOWN, LEFT, RIGHT, ENTER (ENTER is the OK key)"`
		}) (*sdk.CallToolResult, reply, error) {
			key := strings.ToUpper(in.Key)
			if !slices.Contains(tv.Keys, key) {
				return nil, reply{}, fmt.Errorf("unknown key %q", in.Key)
			}
			return withTV(func(t tv.TV) (string, error) { return "Pressed " + key + ".", t.Key(key) })
		})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_type_text", Description: "Type text into the text field selected on the TV, like a search box."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
			Text string `json:"text"`
		}) (*sdk.CallToolResult, reply, error) {
			return withTV(func(t tv.TV) (string, error) { return "Typed.", t.Type(in.Text) })
		})

	sdk.AddTool(s, &sdk.Tool{Name: "tv_notify", Description: "Show a short notification on the TV screen."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
			Message string `json:"message"`
		}) (*sdk.CallToolResult, reply, error) {
			return withTV(func(t tv.TV) (string, error) { return "Shown on the TV.", t.Notify(in.Message) })
		})

	sdk.AddTool(s, &sdk.Tool{Name: "wifi_show_on_tv", Description: "Show the home Wi-Fi QR code full screen on the TV so guests can scan it."},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, reply, error) {
			s, err := home.Load()
			if err != nil {
				return nil, reply{}, err
			}
			if s.Wifi.SSID == "" {
				return nil, reply{}, errors.New("no Wi-Fi saved yet. The user has to run: canape wifi set")
			}
			return withTV(func(t tv.TV) (string, error) {
				return "The Wi-Fi QR is on the TV. Back on the remote closes it.",
					wifi.ShowOnTV(s.Wifi.SSID, s.Wifi.Password, discovery.LocalIP(), t.Open)
			})
		})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "computer_key",
		Description: "Press a key on the computer running canape, usually the one streaming to the TV: pause the movie, skip back or forward, change its volume. macOS only for now.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
		Key string `json:"key" jsonschema:"one of playpause, space, back, forward, fullscreen, escape, volume-up, volume-down, mute"`
	}) (*sdk.CallToolResult, reply, error) {
		key := computer.Key(in.Key)
		if !slices.Contains(computer.Keys, key) {
			return nil, reply{}, fmt.Errorf("unknown key %q", in.Key)
		}
		if err := computer.Press(key); err != nil {
			return nil, reply{}, err
		}
		return nil, reply{"Pressed " + in.Key + " on " + computer.Name() + "."}, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "network_devices",
		Description: "Devices on the home network, from the Freebox: name, IP, MAC, vendor, online now or last seen.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
		All bool `json:"all,omitempty" jsonschema:"also list devices that are offline"`
	}) (*sdk.CallToolResult, devicesOutput, error) {
		out, err := networkDevices(in.All)
		return nil, out, err
	})

	return s.Run(context.Background(), &sdk.StdioTransport{})
}

// withTV connects to the paired TV for one tool call.
func withTV(fn func(tv.TV) (string, error)) (*sdk.CallToolResult, reply, error) {
	s, err := home.Load()
	if err != nil {
		return nil, reply{}, err
	}
	t, err := s.ConnectTV()
	if err != nil {
		return nil, reply{}, err
	}
	defer t.Close()
	msg, err := fn(t)
	return nil, reply{msg}, err
}

type netDevice struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Vendor   string `json:"vendor"`
	Online   bool   `json:"online"`
	LastSeen string `json:"last_seen"`
}

type devicesOutput struct {
	Devices []netDevice `json:"devices"`
}

func networkDevices(all bool) (devicesOutput, error) {
	var out devicesOutput
	s, err := home.Load()
	if err != nil {
		return out, err
	}
	hosts, err := s.Devices(all)
	for _, h := range hosts {
		out.Devices = append(out.Devices, netDevice{
			Name: h.Name, IP: h.IPv4(), MAC: h.MAC(), Vendor: h.Vendor, Online: h.Reachable,
			LastSeen: time.Unix(h.LastActivity, 0).Format(time.RFC3339),
		})
	}
	return out, err
}
