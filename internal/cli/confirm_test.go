package cli

import (
	"errors"
	"strings"
	"testing"
)

// The user's answer at a terminal decides, and --yes applies at once.
func TestConfirmFollowsTheAnswer(t *testing.T) {
	tests := []struct {
		answer   string
		terminal bool
		yes      bool
		want     bool
	}{
		{"y\n", true, false, true},
		{"yes\n", true, false, true},
		{"n\n", true, false, false},
		{"\n", true, false, false},
		{"", true, false, false},
		{"", false, true, true},
	}
	// Check every answer
	for _, tt := range tests {
		var out strings.Builder

		got, err := confirm(strings.NewReader(tt.answer), &out, tt.terminal, tt.yes, "remove")

		// Report an answer read otherwise
		if err != nil || got != tt.want {
			t.Errorf("confirm(%q, terminal %v, yes %v) = %v, %v, want %v", tt.answer, tt.terminal, tt.yes, got, err, tt.want)
		}
	}
}

// Lacking both a terminal and --yes, the command is a usage error that names its action.
func TestConfirmTurnsAwayAMissingTerminal(t *testing.T) {
	var out strings.Builder

	_, err := confirm(strings.NewReader("y\n"), &out, false, false, "remove")

	// Require a usage error naming the action
	if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "pass --yes to remove without a terminal") {
		t.Errorf("confirm() without a terminal = %v, want ErrUsage naming remove", err)
	}
}

// At a terminal, the question names the action.
func TestConfirmAsksByTheActionsName(t *testing.T) {
	var out strings.Builder

	_, _ = confirm(strings.NewReader("n\n"), &out, true, false, "close sooner")

	// Require the action as the question
	if out.String() != "Close sooner? [y/N] " {
		t.Errorf("question = %q, want Close sooner? [y/N]", out.String())
	}
}
