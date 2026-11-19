package tui

import "testing"

// A secret's name that isn't an uppercase env name stays on the screen with the rule it breaks.
func TestFlowSecretHoldsALowercaseName(t *testing.T) {
	m, _ := onHome(t, homeNamed(t, "running"), usualWidth, usualHeight)

	m, _ = drive(m, keys("sec", enter, enter, "lower", enter)...)

	// Require the name held with its rule
	requireLines(t, m, "❯ lower", `✗ "lower" is not an uppercase env name`)
}
