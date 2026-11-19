package tui

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/menu/menutest"
)

// Keys the driver presses beside the ones the tests share.
var (
	up       = tea.KeyPressMsg{Code: tea.KeyUp}
	left     = tea.KeyPressMsg{Code: tea.KeyLeft}
	clearAll = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
)

// onHome is the app on a home, recording every command it builds, each of which echoes and succeeds, the ones for
// the real terminal handed over without running.
func onHome(t *testing.T, h menu.Home, width, height int) (model, *[][]string) {
	t.Helper()
	var ran [][]string
	m := newModel(Config{
		Home: func() menu.Home { return h },
		Command: func(args []string) *exec.Cmd {
			ran = append(ran, args)
			return exec.CommandContext(t.Context(), "echo", append([]string{"ran"}, args...)...) //nolint:gosec // the tests echo the menu's own arguments
		},
		Exec: func(_ *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
			return func() tea.Msg { return fn(nil) }
		},
	})
	m.width, m.height = width, height
	m, _ = settle(m, m.Init())
	return m, &ran
}

// screen is what the app shows, without colors and each line without the padding at its end.
func screen(m model) []string {
	lines := strings.Split(stripped(m.View().Content), "\n")
	// Trim the padding of every line
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return lines
}

// listOf is the list the field the app shows moves through, and nil for a field without one.
func listOf(f field) *list {
	// Take the list of the kind of field
	switch f := f.(type) {
	case choice:
		return &f.list
	case multi:
		return &f.list
	case view:
		_, l := f.current()
		return l
	}
	return nil
}

// moveTo presses ↓ or ↑ until the row with the value is highlighted, and reports whether it got there.
func moveTo(m model, value string) (model, bool) {
	l := listOf(m.field)
	// Report a field without rows
	if l == nil {
		return m, false
	}
	i := slices.IndexFunc(l.shown(), func(o menu.Option) bool { return o.Value == value })
	// Report a value no row holds
	if i < 0 {
		return m, false
	}
	key := down
	// Press ↑ for a row above the cursor
	if i < l.cursor {
		key = up
	}
	// Press until the row is highlighted
	for range abs(i - l.cursor) {
		m, _ = drive(m, key)
	}
	return m, true
}

// abs is the distance of a number from zero.
func abs(n int) int {
	return max(n, -n)
}

// marksTo turns the marks of the list in view until exactly the values given are marked.
func marksTo(m model, values []string) model {
	l := listOf(m.field)
	// Turn every row whose mark differs
	for _, o := range slices.Clone(l.shown()) {
		// Turn a row whose mark differs from the one wanted
		if listOf(m.field).marks[o.Value] != slices.Contains(values, o.Value) {
			m, _ = moveTo(m, o.Value)
			m, _ = drive(m, space)
		}
	}
	return m
}

// answer gives the question in view the answer, the way a user does: the arrows to a row and Enter, the marks with
// Space, y or n, the line typed, the tab with the arrows and its key.
func answer(t *testing.T, m model, values []string) model {
	t.Helper()
	// Answer by the kind of field
	switch f := m.field.(type) {
	case choice:
		m = to(t, m, values[0])
		m, _ = drive(m, enter)
	case multi:
		m = marksTo(m, values)
		m, _ = drive(m, enter)
	case confirm:
		key := "n"
		// Press y for yes
		if values[0] == yes {
			key = "y"
		}
		m, _ = drive(m, typed(key)...)
	case text:
		m, _ = drive(m, append(append([]tea.Msg{clearAll}, typed(values[0])...), enter)...)
	case view:
		m = toTab(t, m, f, values[0])
		m = inTab(t, m, values[1:])
	default:
		t.Fatalf("no way to answer a %T", f)
	}
	return m
}

// to moves to the row with the value, and stops the test where no row holds it.
func to(t *testing.T, m model, value string) model {
	t.Helper()
	m, ok := moveTo(m, value)
	// Stop where no row holds the value
	if !ok {
		t.Fatalf("no row %q on:\n%s", value, strings.Join(screen(m), "\n"))
	}
	return m
}

// toTab switches the view to the tab named.
func toTab(t *testing.T, m model, v view, name string) model {
	t.Helper()
	i := slices.IndexFunc(v.q.Tabs, func(tab menu.Tab) bool { return tab.Name == name })
	// Stop where the view has no such tab
	if i < 0 {
		t.Fatalf("no tab %q on:\n%s", name, strings.Join(screen(m), "\n"))
	}
	key := right
	// Press ← for a tab before the one in view
	if i < v.tab {
		key = left
	}
	// Press until the tab is in view
	for range abs(i - v.tab) {
		m, _ = drive(m, key)
	}
	return m
}

// inTab answers on the tab in view: its key, a new order, its marks, or a row.
func inTab(t *testing.T, m model, values []string) model {
	t.Helper()
	tab, _ := m.field.(view).current()
	k := slices.IndexFunc(tab.Keys, func(k menu.Key) bool { return len(values) > 0 && k.Value == values[0] })
	// Answer the way the tab takes it
	switch {
	case k >= 0:
		m, _ = drive(m, typed(tab.Keys[k].Key)...)
	case len(values) > 1 && values[0] == menu.Order:
		m = to(t, m, values[1])
		m, _ = drive(m, append(typed("m"), down, enter)...)
	case tab.Multi:
		m = marksTo(m, values)
		m, _ = drive(m, enter)
	default:
		m = to(t, m, values[0])
		m, _ = drive(m, enter)
	}
	return m
}

// requireLines requires the screen to show every line given, whole and in the order given, the way lazygit's tests
// assert a view's lines.
func requireLines(t *testing.T, m model, want ...string) {
	t.Helper()
	shown := screen(m)
	at := 0
	// Find every line after the one before it
	for _, w := range want {
		i := slices.Index(shown[at:], w)
		// Stop at a line the screen lacks
		if i < 0 {
			t.Fatalf("screen lacks %q after line %d:\n%s", w, at, strings.Join(shown, "\n"))
		}
		at += i + 1
	}
}

// lacksLine requires the screen to show no line that holds the text given.
func lacksLine(t *testing.T, m model, text string) {
	t.Helper()
	// Check every line of the screen
	for _, line := range screen(m) {
		// Stop at a line that holds the text
		if strings.Contains(line, text) {
			t.Fatalf("screen shows %q:\n%s", text, strings.Join(screen(m), "\n"))
		}
	}
}

// homeNamed is the test home of the name given.
func homeNamed(t *testing.T, name string) menu.Home {
	t.Helper()
	i := slices.IndexFunc(menutest.Homes(), func(h menutest.Home) bool { return h.Name == name })
	// Stop at a home the tests lack
	if i < 0 {
		t.Fatalf("no home %q", name)
	}
	return menutest.Homes()[i].Home
}
