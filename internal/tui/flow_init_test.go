package tui

import "testing"

// Esc on the AppArmor question steps back to the isolation, still on the process the user picked.
func TestFlowInitStepsBackToTheIsolation(t *testing.T) {
	m, _ := onHome(t, homeNamed(t, "outside"), usualWidth, usualHeight)
	m, _ = drive(m, keys("init", enter, "2")...)

	m, _ = drive(m, esc)

	// Require the isolation again, the process highlighted
	requireLines(t, m, "Isolation", "❯ 2. Process            the cage shares this machine's kernel")
}
