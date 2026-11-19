package config

import (
	"fmt"
	"slices"
	"strconv"
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

// ParseFor reads how long a host stays open: seconds alone, or a duration like 90s, 20m, 1h or 1h30m.
func ParseFor(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	// Read a number alone as seconds
	if n, nerr := strconv.Atoi(s); nerr == nil {
		d, err = time.Duration(n)*time.Second, nil
	}
	// Turn away anything that reads as no positive time
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%w duration %q, want seconds or a duration like 90s, 20m, 1h or 1h30m", ErrInvalid, s)
	}
	return d, nil
}

// ClosesAt writes the local clock time a host closes, with its date when that is another day.
func ClosesAt(end string, now time.Time) string {
	t, err := time.Parse(EndTimeLayout, end)
	// Show an end time that fails to read as it stands
	if err != nil {
		return end
	}
	t, now = t.Local(), now.Local()
	// Name the date of an end on another day
	if t.YearDay() != now.YearDay() || t.Year() != now.Year() {
		return t.Format("Jan 2 15:04")
	}
	return t.Format("15:04")
}

// ClosesAtOrNone writes the local clock time a host closes, and nothing for a host that stays open.
func ClosesAtOrNone(end string, now time.Time) string {
	// Keep nothing for a host that stays open
	if end == "" {
		return ""
	}
	return ClosesAt(end, now)
}

// Until writes how long a host stays open, expired once its end has come, and nothing for a host that stays open.
func Until(end string, now time.Time) string {
	t, err := time.Parse(EndTimeLayout, end)
	// Show nothing for a host that stays open
	if end == "" || err != nil {
		return ""
	}
	left := t.Sub(now)
	// Name a host whose end has come
	switch {
	case left <= 0:
		return "expired"
	case left < time.Minute:
		return fmt.Sprintf("in %ds", int(left.Seconds()))
	case left < time.Hour:
		return fmt.Sprintf("in %dm", int(left.Minutes()))
	case left < 24*time.Hour:
		return fmt.Sprintf("in %dh", int(left.Hours()))
	default:
		return fmt.Sprintf("in %dd", int(left.Hours()/24))
	}
}
