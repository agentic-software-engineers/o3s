package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/menu/menutest"
)

// walked follows one path of the menu on the screen, from the main menu to where it ends, handing every screen that
// asks to the check given, and hands back the app where the path ends with the commands it ran.
func walked(t *testing.T, h menu.Home, p menutest.Path, width, height int, check func(m model)) (model, [][]string) {
	t.Helper()
	m, ran := onHome(t, h, width, height)
	// Answer every question of the path on the screen
	for i, a := range p.Answers {
		// Stop where the screen asks another question than the path
		if m.state != asking || m.q.Key != a.Key {
			t.Fatalf("step %d asks %q in state %v, want %q, on:\n%s", i, m.q.Key, m.state, a.Key, strings.Join(screen(m), "\n"))
		}
		check(m)
		m = answer(t, m, a.Values)
	}
	// Check the question a stuck path ends at
	if p.Stuck {
		check(m)
	}
	return m, *ran
}

// Every path of the menu runs on the screen the way the model walks it: the screen asks the path's questions in
// turn, takes its answers by the keys a user presses, and runs the commands its plan names.
func TestEveryPath(t *testing.T) {
	// Walk every home
	for _, h := range menutest.Homes() {
		paths, err := menutest.Walk(h.Home)
		// Stop where the walk through the menu fails
		if err != nil {
			t.Fatalf("%s: %v", h.Name, err)
		}
		// Walk every path of the home on the screen
		for i, p := range paths {
			t.Run(fmt.Sprintf("%s/%03d", h.Name, i), func(t *testing.T) {
				_, ran := walked(t, h.Home, p, usualWidth, usualHeight, func(model) {})

				// Require the commands of the path's plan
				if diff := cmp.Diff(commands(p.Plan), ran); diff != "" {
					t.Errorf("%s: commands (-want +got):\n%s", menutest.Line(p), diff)
				}
			})
		}
	}
}

// commands are the command lines a plan runs, its notes left out.
func commands(plan []menu.Command) [][]string {
	var out [][]string
	// Take every command that runs
	for _, c := range plan {
		// Take a command with arguments
		if len(c.Args) > 0 {
			out = append(out, c.Args)
		}
	}
	return out
}

// Every screen that asks a question shows the same as before, once per home it first shows in, so a change of
// wording, order or layout shows in review.
func TestScreens(t *testing.T) {
	seen := map[string]bool{}
	// Write the screens every home first shows
	for _, h := range menutest.Homes() {
		paths, err := menutest.Walk(h.Home)
		// Stop where the walk through the menu fails
		if err != nil {
			t.Fatalf("%s: %v", h.Name, err)
		}
		var b strings.Builder
		// Walk every path, writing each screen the first time it shows
		for _, p := range paths {
			walked(t, h.Home, p, usualWidth, usualHeight, func(m model) {
				shown := strings.Join(screen(m), "\n")
				// Write a screen once
				if !seen[shown] {
					seen[shown] = true
					fmt.Fprintf(&b, "──── %s\n%s\n\n", m.q.Key, strings.TrimRight(shown, "\n"))
				}
			})
		}
		t.Run(h.Name, func(t *testing.T) {
			golden.RequireEqual(t, b.String())
		})
	}
}
