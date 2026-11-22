package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// An empty or unknown command line, or a command with the wrong arguments, is turned away with the usage.
func TestRunTurnsAwayAnUnknownCommandLine(t *testing.T) {
	tests := map[string][]string{
		"no command":           nil,
		"unknown command":      {"open"},
		"connect lacking port": {"connect", fixture.Host},
		"serve with arguments": {"serve", "extra"},
	}
	// Try each command line on its own
	for label, args := range tests {
		t.Run(label, func(t *testing.T) {
			err := run(t.Context(), args, strings.NewReader(""), io.Discard, io.Discard)

			// Require the usage
			if !errors.Is(err, errUsage) {
				t.Errorf("run(%q) = %v, want errUsage", args, err)
			}
		})
	}
}
