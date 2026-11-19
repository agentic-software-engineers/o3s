package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/menu/menutest"
)

// instant are the commands that run without a confirmation, each because undoing it is as quick as doing it, or
// because it changes nothing in the home.
var instant = map[string]string{
	"cage ide":          "opens an editor, changing nothing",
	"cage shell":        "opens a shell, changing nothing",
	"cage start":        "stopping undoes it",
	"cage stop":         "starting undoes it",
	"feature order":     "moving the row back undoes it",
	"feature configure": "editing the option back undoes it",
	"feature store add": "removing the store undoes it",
	"project add":       "clones a repository the user named",
	"project create":    "creates the repository the user named",
	"init --yes":        "finishes the VM the user started",
	"secret add":        "stores a value for a secret, which removing undoes",
}

// knownBreaks are the breaks of the rules the menu shows today, each named by the finding a fix removes it with.
// Fixing one removes its entry, and a new break fails the test.
var knownBreaks = map[string]string{
	"crumb: On (recommended)": "F21: a crumb carries its label's note",
	"crumb: VM (recommended)": "F21: a crumb carries its label's note",
	"wider than 60: feature":  "F25: the footer overflows, upstream charmbracelet/bubbles#1066",
}

// breaks are every break of the rules on the screens and in the plans of every path of every home, at the sizes
// the menu runs at.
func breaks(t *testing.T) map[string]bool {
	t.Helper()
	found := map[string]bool{}
	// Walk every home at every size
	for _, size := range [][2]int{{usualWidth, usualHeight}, {60, 16}} {
		// Walk every path of every home
		for _, h := range menutest.Homes() {
			paths, err := menutest.Walk(h.Home)
			// Stop where the menu can't be walked
			if err != nil {
				t.Fatalf("%s: %v", h.Name, err)
			}
			// Check every screen and the plan of every path
			for _, p := range paths {
				walked(t, h.Home, p, size[0], size[1], func(m model) {
					// Note every break of a rule on the screen
					for _, b := range screenBreaks(m) {
						found[b] = true
					}
				})
				// Note every command the path runs unconfirmed
				for _, b := range planBreaks(p) {
					found[b] = true
				}
			}
		}
	}
	return found
}

// screenBreaks are the rules a screen breaks: a crumb that shows an inner value or a label's note, a line wider than
// the screen, and a question that offers nothing to pick.
func screenBreaks(m model) []string {
	var out []string
	trail := strings.Join(m.crumbs, " / ")
	// Check every part of the trail, a view's tab and row each apart
	for c := range strings.SplitSeq(trail, " / ") {
		// Note a part that shows an inner value or a label's note
		if strings.HasPrefix(c, "+") || strings.Contains(c, "(") {
			out = append(out, "crumb: "+c)
		}
	}
	// Check every line of the screen
	for _, line := range screen(m) {
		// Note a line wider than the screen
		if ansi.StringWidth(line) > m.width {
			out = append(out, fmt.Sprintf("wider than %d: %s", m.width, trail))
		}
	}
	// Note a choice that offers nothing to pick
	if (m.q.Kind == menu.Select || m.q.Kind == menu.Multi) && len(m.q.Options) == 0 {
		out = append(out, "nothing to pick: "+trail)
	}
	return out
}

// planBreaks are the commands of a path that change the home without the user's Yes, or the name typed back where
// the change deletes the user's own work, and outside the instant ones.
func planBreaks(p menutest.Path) []string {
	var out []string
	yes := slices.ContainsFunc(p.Answers, func(a menu.Answer) bool { return a.Key == "apply" && a.Values[0] == "yes" || a.Key == "typed" })
	// Check every command of the plan
	for _, c := range p.Plan {
		line := strings.Join(c.Args, " ")
		// Note a command that runs unconfirmed and isn't instant
		if len(c.Args) > 0 && !yes && !slices.ContainsFunc(slices.Collect(maps.Keys(instant)), func(i string) bool { return strings.HasPrefix(line, i) }) {
			out = append(out, "unconfirmed: o3s "+line)
		}
	}
	return out
}

// The menu keeps its rules on every screen and in every plan of every path, at every size it runs at: crumbs show
// labels, lines fit the screen, a choice offers something, and a change waits for the user's Yes, except the known
// breaks a fix still has to remove.
func TestRules(t *testing.T) {
	found := breaks(t)

	// Require exactly the known breaks
	if diff := cmp.Diff(slices.Sorted(maps.Keys(knownBreaks)), slices.Sorted(maps.Keys(found))); diff != "" {
		t.Errorf("breaks of the rules (-known +found):\n%s", diff)
	}
}
