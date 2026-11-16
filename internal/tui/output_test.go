package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A line wider than the pane breaks at its words onto further lines, so all of it stays in view.
func TestPaneWrapsALongLineAtItsWords(t *testing.T) {
	var p pane
	p.size(40, 10)
	line := "error: the firewall blocks github.com:22, allow it with: o3s firewall add github.com --port 22"

	p.add(line)

	shown := ansi.Strip(p.View())
	// Require every line to fit the pane
	for l := range strings.SplitSeq(shown, "\n") {
		// Report a line wider than the pane
		if ansi.StringWidth(l) > 40 {
			t.Errorf("line %q is %d wide, want at most 40", l, ansi.StringWidth(l))
		}
	}
	// Require every word of the line to stay whole
	for word := range strings.FieldsSeq(line) {
		// Report a word the pane lost
		if !strings.Contains(shown, word) {
			t.Errorf("pane lacks %q:\n%s", word, shown)
		}
	}
}

// A pane that narrows breaks its lines anew for the new width.
func TestPaneRewrapsWhenItNarrows(t *testing.T) {
	var p pane
	p.size(80, 10)
	p.add("error: the firewall blocks github.com:22, allow it with: o3s firewall add github.com --port 22")

	p.size(30, 10)

	shown := ansi.Strip(p.View())
	// Require every line to fit the narrower pane
	for l := range strings.SplitSeq(shown, "\n") {
		// Report a line wider than the pane
		if ansi.StringWidth(l) > 30 {
			t.Errorf("line %q is %d wide, want at most 30", l, ansi.StringWidth(l))
		}
	}
	// Require the end of the command to stay in view
	if !strings.Contains(shown, "--port 22") {
		t.Errorf("pane lost the end of the line:\n%s", shown)
	}
}

// A listed item too long for the screen stays on its own line and goes on under itself, so it reads as one item.
func TestWrappedHangsAnItemUnderItself(t *testing.T) {
	line := "  + NPM_CONFIG_CACHE=/home/ubuntu/features/ghcr.io/example/features/node/cache"

	got := wrapped(line, 40)

	// Require the item to start on its first line
	if !strings.HasPrefix(got[0], "  + NPM_CONFIG_CACHE=") {
		t.Errorf("first line = %q, want the item started on it", got[0])
	}
	// Require every further line under the item, within the width
	for _, l := range got[1:] {
		// Report a line that leaves the item or the width
		if !strings.HasPrefix(l, "    ") || strings.HasPrefix(l, "     ") || ansi.StringWidth(l) > 40 {
			t.Errorf("further line %q, want it under the item and at most 40 wide", l)
		}
	}
	// Require the whole item across the lines
	if joined := strings.ReplaceAll(strings.Join(got, ""), " ", ""); !strings.Contains(joined, "+NPM_CONFIG_CACHE=/home/ubuntu/features/ghcr.io/example/features/node/cache") {
		t.Errorf("lines %q lose part of the item", got)
	}
}
