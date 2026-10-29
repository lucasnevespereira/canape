package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lucasnevespereira/canape/freebox"
	"github.com/lucasnevespereira/canape/lgtv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// reply is what action tools return: one sentence the model can relay.
type reply struct {
	Message string `json:"message"`
}

type nameInput struct {
	Name string `json:"name" jsonschema:"name as listed by tv_status; loose matching works, e.g. hdmi1, apple, youtube"`
}

// mcpServe exposes the CLI's functions to MCP clients like Claude over stdio.
// Each call reads the state file and connects fresh, like a CLI command, so
// nothing has to keep running between calls.
func mcpServe() error {
	s := mcp.NewServer(&mcp.Implementation{Name: "canape", Version: "0.1.0"}, nil)
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "tv_status",
		Description: "Whether the TV is on, its volume, what's on screen, and its inputs and apps. Call it to learn input and app names.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, tvStatus, error) {
		s, err := loadState()
		if err != nil {
			return nil, tvStatus{}, err
		}
		tv, err := connect(s)
		if err != nil {
			return nil, tvStatus{}, nil // off or unreachable: on=false
		}
		defer tv.Close()
		st, err := statusOf(tv)
		return nil, st, err
	})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_power", Description: "Turn the TV on or off."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
			On bool `json:"on" jsonschema:"true to turn on, false to turn off"`
		}) (*mcp.CallToolResult, reply, error) {
			if in.On {
				s, err := loadState()
				if err != nil {
					return nil, reply{}, err
				}
				if s.TV.MAC == "" {
					return nil, reply{}, errors.New("no MAC address saved for the TV. Run: canape tv pair")
				}
				err = lgtv.Wake(s.TV.MAC, s.TV.IP)
				return nil, reply{"Wake-on-LAN sent. The TV takes a few seconds to start."}, err
			}
			return withTV(func(tv *lgtv.TV) (string, error) { return "TV off.", tv.TurnOff() })
		})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_volume", Description: "Set the TV volume."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
			Level int `json:"level" jsonschema:"volume from 0 to 100"`
		}) (*mcp.CallToolResult, reply, error) {
			return withTV(func(tv *lgtv.TV) (string, error) {
				return fmt.Sprintf("Volume set to %d.", in.Level), tv.SetVolume(in.Level)
			})
		})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_mute", Description: "Mute or unmute the TV."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
			Mute bool `json:"mute" jsonschema:"true to mute, false to unmute"`
		}) (*mcp.CallToolResult, reply, error) {
			return withTV(func(tv *lgtv.TV) (string, error) { return "Done.", tv.SetMute(in.Mute) })
		})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_input", Description: "Switch the TV to an HDMI input, e.g. the Apple TV or a console."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, reply, error) {
			return withTV(func(tv *lgtv.TV) (string, error) {
				inputs, err := tv.Inputs()
				if err != nil {
					return "", err
				}
				var choices []choice
				for _, i := range inputs {
					choices = append(choices, choice{i.ID, i.Label})
				}
				c, err := pick(in.Name, choices)
				if err != nil {
					return "", err
				}
				return "Switched to " + c.label + ".", tv.SwitchInput(c.id)
			})
		})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_launch_app", Description: "Open an app on the TV, e.g. YouTube or Prime Video."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, reply, error) {
			return withTV(func(tv *lgtv.TV) (string, error) {
				apps, err := tv.Apps()
				if err != nil {
					return "", err
				}
				var choices []choice
				for _, a := range apps {
					choices = append(choices, choice{a.ID, a.Title})
				}
				c, err := pick(in.Name, choices)
				if err != nil {
					return "", err
				}
				return "Opened " + c.label + ".", tv.Launch(c.id)
			})
		})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_open_url", Description: "Open a web page in the TV's browser."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
			URL string `json:"url" jsonschema:"http or https URL"`
		}) (*mcp.CallToolResult, reply, error) {
			return withTV(func(tv *lgtv.TV) (string, error) { return "Opened " + in.URL + ".", tv.Open(in.URL) })
		})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_press_key", Description: "Press a remote key on the TV."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
			Key string `json:"key" jsonschema:"one of HOME, BACK, UP, DOWN, LEFT, RIGHT, ENTER (ENTER is the OK key)"`
		}) (*mcp.CallToolResult, reply, error) {
			key := strings.ToUpper(in.Key)
			switch key {
			case "HOME", "BACK", "UP", "DOWN", "LEFT", "RIGHT", "ENTER":
			default:
				return nil, reply{}, fmt.Errorf("unknown key %q", in.Key)
			}
			return withTV(func(tv *lgtv.TV) (string, error) { return "Pressed " + key + ".", tv.Button(key) })
		})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_type_text", Description: "Type text into the text field selected on the TV, like a search box."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
			Text string `json:"text"`
		}) (*mcp.CallToolResult, reply, error) {
			return withTV(func(tv *lgtv.TV) (string, error) { return "Typed.", tv.Type(in.Text) })
		})

	mcp.AddTool(s, &mcp.Tool{Name: "tv_notify", Description: "Show a short notification on the TV screen."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
			Message string `json:"message"`
		}) (*mcp.CallToolResult, reply, error) {
			return withTV(func(tv *lgtv.TV) (string, error) { return "Shown on the TV.", tv.Toast(in.Message) })
		})

	mcp.AddTool(s, &mcp.Tool{Name: "wifi_show_on_tv", Description: "Show the home Wi-Fi QR code full screen on the TV so guests can scan it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, reply, error) {
			s, err := loadState()
			if err != nil {
				return nil, reply{}, err
			}
			if s.Wifi.SSID == "" {
				return nil, reply{}, errors.New("no Wi-Fi saved yet. The user has to run: canape wifi set")
			}
			return nil, reply{"The Wi-Fi QR is on the TV. Back on the remote closes it."}, wifiOnTV(s)
		})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "network_devices",
		Description: "Devices on the home network, from the Freebox: name, IP, MAC, vendor, online now or last seen.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		All bool `json:"all,omitempty" jsonschema:"also list devices that are offline"`
	}) (*mcp.CallToolResult, devicesOutput, error) {
		out, err := networkDevices(in.All)
		return nil, out, err
	})

	return s.Run(context.Background(), &mcp.StdioTransport{})
}

// withTV connects to the paired TV for one tool call.
func withTV(fn func(*lgtv.TV) (string, error)) (*mcp.CallToolResult, reply, error) {
	s, err := loadState()
	if err != nil {
		return nil, reply{}, err
	}
	tv, err := connect(s)
	if err != nil {
		return nil, reply{}, err
	}
	defer tv.Close()
	msg, err := fn(tv)
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
	s, err := loadState()
	if err != nil {
		return out, err
	}
	if s.Freebox.AppToken == "" {
		return out, errors.New("not paired with the Freebox. The user has to run: canape freebox pair")
	}
	fbx, err := freebox.New(s.Freebox.AppToken)
	if err != nil {
		return out, err
	}
	hosts, err := fbx.Hosts()
	if err != nil {
		return out, err
	}
	for _, h := range hosts {
		if !h.Reachable && !all {
			continue
		}
		out.Devices = append(out.Devices, netDevice{
			Name: h.Name, IP: h.IPv4(), MAC: h.MAC(), Vendor: h.Vendor, Online: h.Reachable,
			LastSeen: time.Unix(h.LastActivity, 0).Format(time.RFC3339),
		})
	}
	return out, nil
}
