package tui

import (
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// A project's removal holds a name typed back wrong, so the folder stays.
func TestFlowProjectHoldsAWrongName(t *testing.T) {
	m, ran := onHome(t, homeNamed(t, "running"), usualWidth, usualHeight)

	m, _ = drive(m, keys("proj", enter, down, down, enter, enter, fixture.OtherRepo, enter)...)

	// Require the name held
	requireLines(t, m, "Type "+fixture.Repo+" to confirm", "❯ "+fixture.OtherRepo)
	// Report a removal run on a wrong name
	if len(*ran) != 0 {
		t.Errorf("ran %v, want nothing", *ran)
	}
}
