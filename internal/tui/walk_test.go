package tui

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/menu/menutest"
)

// walkSteps is how many actions a walk takes from the screen it starts at.
const walkSteps = 30

// navigation are the keys a walk presses on any screen.
var navigation = []tea.Msg{
	up, down, left, right, enter, esc,
	tea.KeyPressMsg{Code: tea.KeyTab},
	tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift},
	tea.KeyPressMsg{Code: tea.KeyPgDown},
	tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl},
	tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl},
}

// letters are the keys a walk presses where nothing is typed yet, each a key the menu acts on or the first letter of
// a search, so a screen never fills with letters.
var letters = []tea.Msg{
	space, typed("/")[0], typed("m")[0], typed("y")[0], typed("n")[0],
	typed("1")[0], typed("2")[0], typed("3")[0], typed("4")[0],
}

// typing reports whether the screen takes letters as text: a line of text, a search open, or a list already
// filtered.
func typing(m model) bool {
	// Read the field in front
	switch f := m.field.(type) {
	case text:
		return true
	case view:
		return f.searching || listOf(f).filter != ""
	case choice, multi:
		return listOf(f).filter != ""
	}
	return false
}

// actions are what a walk does on a screen: every navigation key, the letters where nothing is typed yet, every key
// the view in front acts on, and on a line of text the line a test types for it.
func actions(m model) [][]tea.Msg {
	out := make([][]tea.Msg, 0, len(navigation)+len(letters)+2)
	// Press every navigation key
	for _, k := range navigation {
		out = append(out, []tea.Msg{k})
	}
	// Press every letter where the screen takes no text yet
	for _, k := range letters {
		// Leave letters out where they would pile up as text
		if m.state != asking || !typing(m) {
			out = append(out, []tea.Msg{k})
		}
	}
	// Press every key the view's tab in front acts on, where it takes no text
	if v, isView := m.field.(view); isView && m.state == asking && !typing(m) {
		tab, _ := v.current()
		// Press every key of the tab
		for _, k := range tab.Keys {
			out = append(out, typed(k.Key))
		}
	}
	// Type the line a test types for the question in front
	if _, isText := m.field.(text); isText && m.state == asking {
		lines, ok := menutest.Typed(m.q.Key)
		// Type the secret's value on a hidden line
		if m.q.Kind == menu.Hidden {
			lines, ok = []string{fixture.SecretValue}, true
		}
		// Type the first line for the question
		if ok {
			out = append(out, keys(clearAll, lines[0]))
		}
	}
	return out
}

// randomWalk answers a path up to the step its seed picks, then takes seeded random actions, handing the screen after
// each to the check given with the keys pressed so far, until the walk ends or leaves the menu. The same seed walks the
// same keys on every run.
func randomWalk(t *testing.T, h menu.Home, p menutest.Path, seed int, check func(m model, pressed []string)) {
	t.Helper()
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed))) //nolint:gosec // a seeded walk repeats the same keys on every run, its seed a path's number
	m, _ := onHome(t, h, usualWidth, usualHeight)
	// Answer the path up to the step the seed picks
	for _, a := range p.Answers[:rng.IntN(len(p.Answers)+1)] {
		m = answer(t, m, a.Values)
	}
	var pressed []string
	// Take one action after another, checking the screen after each
	for range walkSteps {
		acts := actions(m)
		act := acts[rng.IntN(len(acts))]
		pressed = append(pressed, fmt.Sprint(act))
		var quit bool
		m, quit = drive(m, act...)
		// End the walk where it leaves the menu
		if quit {
			return
		}
		check(m, pressed)
	}
}

// Seeded random keys keep the menu to its rules after every key, the way crush's oracle test walks its model, each
// walk starting at a screen of a path of the menu, so the walks reach every part of it, the same keys on every run.
func TestRandomWalks(t *testing.T) {
	// Walk from the paths of every home
	for _, h := range menutest.Homes() {
		paths, err := menutest.Walk(h.Home)
		// Stop where the menu can't be walked
		if err != nil {
			t.Fatalf("%s: %v", h.Name, err)
		}
		// Walk from a screen of every path, the path's number its seed
		for i, p := range paths {
			t.Run(fmt.Sprintf("%s/%03d", h.Name, i), func(t *testing.T) {
				randomWalk(t, h.Home, p, i, func(m model, pressed []string) {
					// Report every break of the rules outside the known ones
					for _, b := range screenBreaks(m) {
						// Report a break no finding names
						if _, known := knownBreaks[b]; !known {
							t.Fatalf("%s from %s after %v, on:\n%s", b, menutest.Line(p), pressed, strings.Join(screen(m), "\n"))
						}
					}
				})
			})
		}
	}
}
