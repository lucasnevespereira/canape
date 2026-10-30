package home

import (
	"context"
	"time"

	"github.com/lucasnevespereira/canape/internal/tv"
	"github.com/lucasnevespereira/canape/internal/tv/lg"
)

// A driver is how canape finds and connects to one brand of TV. Adding a
// brand means a package under internal/tv/ whose connection satisfies tv.TV,
// and one entry here. See docs/adding-a-tv.md.
type driver struct {
	name     string // shown to the user
	discover func(timeout time.Duration) (ip string, err error)
	// connect dials and authenticates. With an empty or forgotten key the TV
	// asks the user to accept canape, and the key to save comes back.
	connect func(ctx context.Context, ip, key string, prompted func()) (tv.TV, string, error)
}

// drivers is keyed by the brand saved in each home's TV.
var drivers = map[string]driver{
	"lg": {
		name:     "LG webOS",
		discover: lg.Discover,
		connect: func(ctx context.Context, ip, key string, prompted func()) (tv.TV, string, error) {
			t, key, err := lg.Connect(ctx, ip, key, prompted)
			if err != nil {
				return nil, "", err // a nil *lg.TV in a tv.TV wouldn't compare to nil
			}
			return t, key, nil
		},
	},
}

// BrandName is how a saved brand reads to the user, e.g. "LG webOS".
func BrandName(brand string) string {
	if d, ok := drivers[brand]; ok {
		return d.name
	}
	return brand
}
