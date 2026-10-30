package tv

import "errors"

// TV is what canape does with any TV. Each brand has a driver package
// (internal/tv/lg, ...) whose connection satisfies it. A driver that can't
// do something returns ErrUnsupported, so the user gets a clear answer.
type TV interface {
	TurnOff() error

	Volume() (Volume, error)
	SetVolume(level int) error // 0 to 100
	SetMute(mute bool) error

	Inputs() ([]Input, error)
	SwitchInput(id string) error
	// Apps lists the apps the user would see in the TV's launcher.
	Apps() ([]App, error)
	Launch(appID string) error
	// Current returns the ID of the input or app on screen.
	Current() (string, error)

	// Open shows a URL in the TV's web browser.
	Open(url string) error
	// Key presses a remote key: one of the Key* constants.
	Key(name string) error
	// Type inserts text into the field that has focus on screen.
	Type(text string) error
	// Notify shows a short message on screen.
	Notify(message string) error

	Close() error
}

// Remote keys every driver maps to its own codes.
const (
	KeyHome  = "HOME"
	KeyBack  = "BACK"
	KeyUp    = "UP"
	KeyDown  = "DOWN"
	KeyLeft  = "LEFT"
	KeyRight = "RIGHT"
	KeyOK    = "ENTER"
)

var Keys = []string{KeyHome, KeyBack, KeyUp, KeyDown, KeyLeft, KeyRight, KeyOK}

var ErrUnsupported = errors.New("this TV doesn't support that")

type Volume struct {
	Level int  `json:"level"`
	Muted bool `json:"muted"`
}

type Input struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Connected bool   `json:"connected"` // something is plugged in
}

type App struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
