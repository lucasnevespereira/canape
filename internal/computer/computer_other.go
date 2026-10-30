//go:build !darwin

package computer

// ponytail: macOS only for now. Linux can use playerctl and xdotool, Windows
// the media virtual keys. Add them when someone runs canape there.

func Name() string { return hostname() }

func Press(Key) error { return ErrUnsupported }

func GetVolume() (Volume, error) { return Volume{}, ErrUnsupported }
