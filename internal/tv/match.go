// Package tv holds what every TV brand shares.
package tv

import (
	"fmt"
	"strings"
)

// Choice is something a user picks by name: an input or an app.
type Choice struct{ ID, Label string }

// Match finds the choice the user meant: an exact id or label first, then a
// unique partial match. So "hdmi1", "HDMI_1" and "apple" all find HDMI 1.
func Match(query string, choices []Choice) (Choice, error) {
	q := norm(query)
	var partial []Choice
	for _, c := range choices {
		if norm(c.ID) == q || norm(c.Label) == q {
			return c, nil
		}
		if strings.Contains(norm(c.ID), q) || strings.Contains(norm(c.Label), q) {
			partial = append(partial, c)
		}
	}
	switch len(partial) {
	case 1:
		return partial[0], nil
	case 0:
		return Choice{}, fmt.Errorf("nothing matches %q", query)
	}
	var labels []string
	for _, c := range partial {
		labels = append(labels, c.Label)
	}
	return Choice{}, fmt.Errorf("%q matches several: %s", query, strings.Join(labels, ", "))
}

func norm(s string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "", "+", "", ".", "").Replace(s))
}
