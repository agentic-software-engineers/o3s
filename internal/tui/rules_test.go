package tui

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/menu/menutest"
	"github.com/hansehart/o3s/internal/tui/screentest"
)

// instant are the commands that run at once, each because undoing it is as quick as doing it, or because it leaves
// the home as it is.
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
	"no footer: update / 3 marked / yes":                   "a running command names no key, though ctrl+c leaves",
	"taller than 14: feature":                              "the feature view runs past a short terminal, its footer out of sight",
	"wider than 60: firewall":                              "a line of the firewall view runs past a narrow terminal",
	"no footer: update / unmarks 1 / yes":                  "a running command names no key, though ctrl+c leaves",
	"taller than 14: firewall / Allowed / api.example.com": "a host's view runs past a short terminal, its footer out of sight",
	"taller than 14: firewall / Allowed / example.com":     "a host's view runs past a short terminal, its footer out of sight",
	"taller than 14: firewall / Allowed / example.net":     "a host's view runs past a short terminal, its footer out of sight",
	"taller than 14: firewall / Allowed / example.org":     "a host's view runs past a short terminal, its footer out of sight",
}

// breaks are every break of the rules on the screens of every path of every home, the screens paths end at and the
// seeded walks from them included, and in the plans, at the sizes the menu runs at.
func breaks(t *testing.T) map[string]bool {
	t.Helper()
	found := map[string]bool{}
	// Walk every home at every size
	for _, size := range [][2]int{{usualWidth, usualHeight}, {60, 16}, {usualWidth, 14}} {
		// Walk every path of every home
		for _, h := range menutest.Homes() {
			paths, err := menutest.Walk(h.Home)
			// Stop where the walk through the menu fails
			if err != nil {
				t.Fatalf("%s: %v", h.Name, err)
			}
			// Check every screen and the plan of every path
			for i, p := range paths {
				m, _ := walked(t, h.Home, p, size[0], size[1], func(m model) {
					// Note every break of a rule on the screen
					for _, b := range screenBreaks(m) {
						found[b] = true
					}
				})
				// Note every break of a rule on the screen the path ends at
				for _, b := range screenBreaks(m) {
					found[b] = true
				}
				// Note every break of a rule a seeded walk from the path meets, at the usual size alone as the walks run
				if size == [2]int{usualWidth, usualHeight} {
					randomWalk(t, h.Home, p, i, func(m model, _ []string) {
						// Note every break of a rule on the screen
						for _, b := range screenBreaks(m) {
							found[b] = true
						}
					})
				}
				// Note every command the path runs unconfirmed
				for _, b := range planBreaks(p) {
					found[b] = true
				}
			}
		}
	}
	return found
}

// screenBreaks are the rules a screen breaks: a crumb that shows an inner value or a label's note, the rules of the
// screen's text, and a question with an empty list to pick from.
func screenBreaks(m model) []string {
	var out []string
	trail := strings.Join(m.crumbs(), " / ")
	// Check every part of the trail, a view's tab and row each apart
	for c := range strings.SplitSeq(trail, " / ") {
		// Note a part that shows an inner value or a label's note
		if strings.HasPrefix(c, "+") || strings.Contains(c, "(") {
			out = append(out, "crumb: "+c)
		}
	}
	// Note every rule the screen's text breaks
	for _, b := range screentest.Breaks(screen(m), m.width, m.height) {
		out = append(out, b+": "+trail)
	}
	// Note a choice with an empty list to pick from
	if (m.q.Kind == menu.Select || m.q.Kind == menu.Multi) && len(m.q.Options) == 0 {
		out = append(out, "nothing to pick: "+trail)
	}
	return out
}

// planBreaks are the commands of a path that change the home before the user's Yes, or the name typed back where
// the change deletes the user's own work, and outside the instant ones.
func planBreaks(p menutest.Path) []string {
	var out []string
	yes := slices.ContainsFunc(p.Answers, func(a menu.Answer) bool { return a.Key == "apply" && a.Values[0] == "yes" || a.Key == "typed" })
	// Check every command of the plan
	for _, c := range p.Plan {
		line := strings.Join(c.Args, " ")
		// Note a command outside the instant ones that runs unconfirmed
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
