package cli

import (
	"errors"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
)

// A duration reads as seconds alone or in Go's form, and anything else as no time.
func TestParseForReadsSecondsAndDurations(t *testing.T) {
	tests := map[string]time.Duration{"90": 90 * time.Second, "90s": 90 * time.Second, "20m": 20 * time.Minute, "1h": time.Hour, "1h30m": 90 * time.Minute}
	// Read every form
	for in, want := range tests {
		got, err := parseFor(in)

		// Report a form read wrong
		if err != nil || got != want {
			t.Errorf("parseFor(%q) = %v, %v, want %v", in, got, err, want)
		}
	}
	// Turn away what reads as no positive time
	for _, in := range []string{"", "soon", "0", "-5m", "1d"} {
		_, err := parseFor(in)

		// Report a bad duration taken as a time
		if !errors.Is(err, ErrUsage) {
			t.Errorf("parseFor(%q) = %v, want %v", in, err, ErrUsage)
		}
	}
}

// A new end narrows a host that stays open, and one whose end comes later, and nothing else.
func TestNarrowsComparesTheEnds(t *testing.T) {
	tests := []struct {
		old, next string
		want      bool
	}{
		{"", fixture.InAnHourText, true},
		{fixture.InTwoHoursText, fixture.InAnHourText, true},
		{fixture.InAnHourText, fixture.InTwoHoursText, false},
		{fixture.InAnHourText, "", false},
		{"", "", false},
	}
	// Judge every pair of ends
	for _, tt := range tests {
		got := narrows(tt.old, tt.next)

		// Report a pair judged wrong
		if got != tt.want {
			t.Errorf("narrows(%q, %q) = %v, want %v", tt.old, tt.next, got, tt.want)
		}
	}
}

// The time left reads in its largest whole unit, expired once the end has come, and empty without an end.
func TestUntilWritesTheTimeLeft(t *testing.T) {
	after := func(d time.Duration) string { return fixture.Now.Add(d).Format(time.RFC3339) }
	tests := map[string]string{"": "", after(-time.Hour): "expired", after(30 * time.Second): "in 30s", after(52 * time.Minute): "in 52m", after(3 * time.Hour): "in 3h", after(72 * time.Hour): "in 3d"}
	// Write every end
	for end, want := range tests {
		// Report an end written wrong
		if got := until(end, fixture.Now); got != want {
			t.Errorf("until(%q) = %q, want %q", end, got, want)
		}
	}
}
