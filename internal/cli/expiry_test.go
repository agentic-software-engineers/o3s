package cli

import (
	"errors"
	"testing"
	"time"
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
		{"", "2026-11-16T19:00:00Z", true},
		{"2026-11-16T20:00:00Z", "2026-11-16T19:00:00Z", true},
		{"2026-11-16T19:00:00Z", "2026-11-16T20:00:00Z", false},
		{"2026-11-16T19:00:00Z", "", false},
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
	now := time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)
	tests := map[string]string{"": "", "2026-11-16T17:00:00Z": "expired", "2026-11-16T18:00:30Z": "in 30s", "2026-11-16T18:52:00Z": "in 52m", "2026-11-16T21:00:00Z": "in 3h", "2026-11-19T18:00:00Z": "in 3d"}
	// Write every end
	for end, want := range tests {
		// Report an end written wrong
		if got := until(end, now); got != want {
			t.Errorf("until(%q) = %q, want %q", end, got, want)
		}
	}
}
