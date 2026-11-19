package tui

import (
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// A host line the firewall can't read stays on the screen with the rule it breaks, and the fixed line goes on.
func TestFlowFirewallHoldsAnUnreadableHost(t *testing.T) {
	m, _ := onHome(t, homeNamed(t, "running"), usualWidth, usualHeight)
	m, _ = drive(m, keys("fir", enter, enter, fixture.NewHost+" bad_name", enter)...)

	// Require the line held with its rule
	requireLines(t, m, "❯ "+fixture.NewHost+" bad_name", `✗ "bad_name" is not a host name`)

	m, _ = drive(m, keys(clearAll, fixture.NewHost, enter)...)

	// Require the ports next
	requireLines(t, m, "Ports")
}
