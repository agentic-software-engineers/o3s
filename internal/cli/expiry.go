package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hansehart/o3s/internal/config"
)

// parseFor reads how long a host stays open, a duration the allowlist turns away being a usage error.
func parseFor(s string) (time.Duration, error) {
	d, err := config.ParseFor(s)
	// Mark a duration that reads as no time as a fault of the command line
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrUsage, err)
	}
	return d, nil
}

// narrows reports whether a new end time closes a host sooner than it would close now, an end on a host that stays open included.
func narrows(old, next string) bool {
	return next != "" && (old == "" || next < old)
}

// closesAt writes the local clock time a host closes, with its date when that is another day.
func closesAt(end string, now time.Time) string {
	t, err := time.Parse(config.EndTimeLayout, end)
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

// closesAtOrNone writes the local clock time a host closes, and nothing for a host that stays open.
func closesAtOrNone(end string, now time.Time) string {
	// Keep nothing for a host that stays open
	if end == "" {
		return ""
	}
	return closesAt(end, now)
}

// until writes how long a host stays open, expired once its end has come, and nothing for a host that stays open.
func until(end string, now time.Time) string {
	t, err := time.Parse(config.EndTimeLayout, end)
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

// consequences names what a sooner end does to a host and to everyone who needs it.
func consequences(out io.Writer, h config.Host, forValue, next string, now time.Time) {
	needed := joinAnd(h.By)
	// Name the end a host that stays open would get, for everyone who needs it
	if h.Expires == "" {
		fmt.Fprintf(out, "%s is allowed with no end time, needed by %s.\n", h.Name, needed)
		whom := ""
		// Name that the end reaches every one of several who need the host
		if len(h.By) > 1 {
			whom = " for all of them"
		}
		fmt.Fprintf(out, "With --for %s it closes at %s%s.\n", forValue, closesAt(next, now), whom)
		return
	}
	fmt.Fprintf(out, "%s closes at %s, needed by %s.\n", h.Name, closesAt(h.Expires, now), needed)
	fmt.Fprintf(out, "With --for %s it closes at %s instead.\n", forValue, closesAt(next, now))
}

// joinAnd joins names the way a sentence lists them.
func joinAnd(names []string) string {
	// Join a single name as it stands, and the last of several with and
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
