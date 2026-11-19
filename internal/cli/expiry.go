package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/words"
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

// consequences names what a sooner end does to a host and to everyone who needs it.
func consequences(out io.Writer, h config.Host, forValue, next string, now time.Time) {
	needed := words.JoinAnd(h.By)
	// Name the end a host that stays open would get, for everyone who needs it
	if h.Expires == "" {
		fmt.Fprintf(out, "%s is allowed with no end time, needed by %s.\n", h.Name, needed)
		whom := ""
		// Name that the end reaches every one of several who need the host
		if len(h.By) > 1 {
			whom = " for all of them"
		}
		fmt.Fprintf(out, "With --for %s it closes at %s%s.\n", forValue, config.ClosesAt(next, now), whom)
		return
	}
	fmt.Fprintf(out, "%s closes at %s, needed by %s.\n", h.Name, config.ClosesAt(h.Expires, now), needed)
	fmt.Fprintf(out, "With --for %s it closes at %s instead.\n", forValue, config.ClosesAt(next, now))
}
