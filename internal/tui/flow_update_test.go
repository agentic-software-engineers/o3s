package tui

import (
	"testing"

	"github.com/hansehart/o3s/internal/menu"
)

// An update without the cli confirms the parts left marked and runs without restarting the menu.
func TestFlowUpdateWithoutTheCli(t *testing.T) {
	m, ran := onHome(t, homeNamed(t, "running"), usualWidth, usualHeight)
	m, _ = drive(m, keys("upd", enter, space, enter)...)

	// Require the cli left out of the confirmation
	requireLines(t, m, "~ stack   1 file, 2 images", "Updates 2 parts")
	lacksLine(t, m, "~ "+menu.CLI)

	m, quit := drive(m, typed("y")...)

	// Require the update done
	requireLines(t, m, "✓ done")
	// Report a menu that left or ran anything but the update
	if quit || len(*ran) != 1 {
		t.Errorf("quit %v, ran %v, want one update and the menu open", quit, *ran)
	}
}
