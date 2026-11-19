package tui

import "testing"

// Enter after a command goes back to the cage's view on the action it ran, Esc after it to the main menu.
func TestFlowCageGoesBackAfterACommand(t *testing.T) {
	m, _ := onHome(t, homeNamed(t, "running"), usualWidth, usualHeight)
	m, _ = drive(m, keys("cage", enter, down, down, enter)...)

	// Require the stop's output and the ways back
	requireLines(t, m, "$ o3s cage stop", "ran cage stop", "✓ done")

	back, _ := drive(m, enter)
	menu, _ := drive(m, esc)

	// Require the cage's view on the action it ran on Enter, and the main menu on Esc
	requireLines(t, back, "cage", "❯ Stop")
	requireLines(t, menu, "What do you want to do?")
}
