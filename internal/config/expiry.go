package config

import (
	"slices"
	"time"
)

// EndTimeLayout is the one shape an end time takes, UTC with whole seconds, which the gateway and the proxy compare as text.
const EndTimeLayout = "2006-01-02T15:04:05Z"

// EndTime writes the moment a host closes in the shape the allowlist keeps.
func EndTime(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(EndTimeLayout)
}

// validEndTime reports whether an end time takes the one shape the allowlist keeps.
func validEndTime(s string) bool {
	t, err := time.Parse(EndTimeLayout, s)
	return err == nil && t.Format(EndTimeLayout) == s
}

// Expired reports whether a host's end time has come, and never for a host that stays open.
func (h Host) Expired(now time.Time) bool {
	end, err := time.Parse(EndTimeLayout, h.Expires)
	return h.Expires != "" && err == nil && !now.Before(end)
}

// Expired lists the hosts whose end time has come, in the order the allowlist holds them.
func (c Config) Expired(now time.Time) []Host {
	var out []Host
	// Take every host whose end time has come
	for _, h := range c.Hosts {
		// Take a host that has closed
		if h.Expired(now) {
			out = append(out, h)
		}
	}
	return out
}

// Prune takes every expired host off the allowlist and hands back their names.
func (c *Config) Prune(now time.Time) []string {
	var pruned []string
	c.Hosts = slices.DeleteFunc(c.Hosts, func(h Host) bool {
		// Name a host that leaves
		if h.Expired(now) {
			pruned = append(pruned, h.Name)
			return true
		}
		return false
	})
	return pruned
}
