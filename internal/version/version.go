// Package version says which canape is running.
package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Version is stamped in at release time:
//
//	go build -ldflags "-X github.com/lucasnevespereira/canape/internal/version.Version=0.1.0"
//
// Without it, Get falls back to what Go recorded: the module version for
// `go install ...@v0.1.0`, otherwise "dev".
var Version = ""

// tag matches release tags. Since Go 1.24 a local `go build` records a
// pseudo-version like v0.0.0-20261030164101-791ca02f45c6+dirty, which isn't one.
var tag = regexp.MustCompile(`^v\d+\.\d+\.\d+(-(alpha|beta|rc)\.\d+)?$`)

// Get returns the version number alone, e.g. "0.1.0" or "dev".
func Get() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && tag.MatchString(info.Main.Version) {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}

// String is what `canape version` prints, e.g. "canape 0.1.0 (4f4f481, 2026-10-30)".
// The commit and date come from Go's build info, so a local `go build` shows
// them too.
func String() string {
	s := "canape " + Get()
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return s
	}
	var rev, date, dirty string
	for _, kv := range info.Settings {
		switch kv.Key {
		case "vcs.revision":
			rev = kv.Value[:min(7, len(kv.Value))]
		case "vcs.time":
			date = kv.Value[:min(10, len(kv.Value))]
		case "vcs.modified":
			if kv.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return s
	}
	if date != "" {
		date = ", " + date
	}
	return s + " (" + rev + dirty + date + ")"
}
