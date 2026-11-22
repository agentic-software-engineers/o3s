package tui

import (
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// An option's edit saves at once and lands back on the options view at the option's row.
func TestFlowFeatureOptionEditLandsOnItsRow(t *testing.T) {
	m, ran := onHome(t, homeNamed(t, "running"), usualWidth, usualHeight)
	m, _ = drive(m, keys("feat", enter, enter, enter, down, enter, "1")...)

	// Require the options view again, on the mode
	requireLines(t, m, "Changes apply on the next rebuild.", "❯ "+fixture.Choice+"      "+fixture.OtherChoiceValue+"   "+fixture.ChoiceValue, "    "+fixture.ChoiceAbout)
	// Report anything but one configure run
	if len(*ran) != 1 {
		t.Errorf("ran %v, want one configure", *ran)
	}
}

// Moving a feature and dropping it where it was stays idle, Esc puts a moved one back.
func TestFlowFeatureRowMovesBack(t *testing.T) {
	m, ran := onHome(t, homeNamed(t, "running"), usualWidth, usualHeight)
	m, _ = drive(m, keys("feat", enter, "m", down)...)

	// Require the row carried below the other
	requireLines(t, m, "  "+fixture.OtherFeature+"     "+fixture.Major+"     "+fixture.Store, "↕ "+fixture.Feature+"      "+fixture.Major+"     "+fixture.Store+"   "+fixture.OtherHost)

	m, _ = drive(m, esc)

	// Require the first order back
	requireLines(t, m, "❯ "+fixture.Feature+"      "+fixture.Major+"     "+fixture.Store+"   "+fixture.OtherHost, "  "+fixture.OtherFeature+"     "+fixture.Major+"     "+fixture.Store)
	// Report a command run for a row put back
	if len(*ran) != 0 {
		t.Errorf("ran %v, want nothing", *ran)
	}
}
