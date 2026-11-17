package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/hansehart/o3s/internal/fixture"
)

// blockedHint is an error wider than a usual screen, which ends in the command a user copies.
var blockedHint = fmt.Sprintf("error: the firewall blocks %s:%d, allow it with: o3s firewall add %s --port %d", fixture.Host, fixture.OtherPort, fixture.Host, fixture.OtherPort)

// A line wider than the pane breaks at its words onto further lines, so all of it stays in view.
func TestPaneWrapsALongLineAtItsWords(t *testing.T) {
	var p pane
	p.size(narrow, paneLines)
	line := blockedHint

	p.add(line)

	shown := ansi.Strip(p.View())
	// Require every line to fit the pane
	for l := range strings.SplitSeq(shown, "\n") {
		// Report a line wider than the pane
		if ansi.StringWidth(l) > narrow {
			t.Errorf("line %q is %d wide, want at most %d", l, ansi.StringWidth(l), narrow)
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
	p.size(usualWidth, paneLines)
	p.add(blockedHint)

	p.size(narrower, paneLines)

	shown := ansi.Strip(p.View())
	// Require every line to fit the narrower pane
	for l := range strings.SplitSeq(shown, "\n") {
		// Report a line wider than the pane
		if ansi.StringWidth(l) > narrower {
			t.Errorf("line %q is %d wide, want at most %d", l, ansi.StringWidth(l), narrower)
		}
	}
	// Require the end of the command to stay in view
	if !strings.Contains(shown, fmt.Sprintf("--port %d", fixture.OtherPort)) {
		t.Errorf("pane lost the end of the line:\n%s", shown)
	}
}

// A listed item too long for the screen stays on its own line and goes on under itself, so it reads as one item.
func TestWrappedHangsAnItemUnderItself(t *testing.T) {
	item := "NPM_CONFIG_CACHE=/home/ubuntu/features/" + fixture.Store + "/" + fixture.Feature + "/cache"
	line := "  + " + item

	got := wrapped(line, narrow)

	// Require the item to start on its first line
	if !strings.HasPrefix(got[0], "  + NPM_CONFIG_CACHE=") {
		t.Errorf("first line = %q, want the item started on it", got[0])
	}
	// Require every further line under the item, within the width
	for _, l := range got[1:] {
		// Report a line that leaves the item or the width
		if !strings.HasPrefix(l, "    ") || strings.HasPrefix(l, "     ") || ansi.StringWidth(l) > narrow {
			t.Errorf("further line %q, want it under the item and at most %d wide", l, narrow)
		}
	}
	// Require the whole item across the lines
	if joined := strings.ReplaceAll(strings.Join(got, ""), " ", ""); !strings.Contains(joined, "+"+item) {
		t.Errorf("lines %q lose part of the item", got)
	}
}
