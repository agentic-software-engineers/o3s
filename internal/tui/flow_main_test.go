package tui

import "testing"

// Typing on the main menu narrows it to the groups that match, and Esc brings every group back before it leaves.
func TestFlowMainMenuSearches(t *testing.T) {
	m, _ := onHome(t, homeNamed(t, "running"), usualWidth, usualHeight)

	m, _ = drive(m, typed("fi")...)

	// Require the firewall alone, highlighted
	requireLines(t, m, "❯ firewall   hosts the cage reaches · 4 allowed · 1 blocked")
	lacksLine(t, m, "feature    tools in the cage")

	m, quit := drive(m, esc)

	// Require every group back
	requireLines(t, m, "❯ cage       VM, stack and cage · running", "  update     the cli, the stack and the features")
	// Report a menu that left on clearing the search
	if quit {
		t.Error("esc on a search left the menu")
	}
}
