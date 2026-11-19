package menutest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/hansehart/o3s/internal/menu"
)

// Every path of the menu on every home reads as one line from the main menu to the commands it ends in, so a path
// that comes, goes or changes shows in review.
func TestPaths(t *testing.T) {
	var b strings.Builder

	// Write the paths of every home
	for _, h := range Homes() {
		paths, err := Walk(h.Home)
		// Report a menu the walk can't follow
		if err != nil {
			t.Fatalf("%s: %v", h.Name, err)
		}
		fmt.Fprintf(&b, "# %s, %d paths\n", h.Name, len(paths))
		// Write every path as a line
		for _, p := range paths {
			fmt.Fprintln(&b, Line(p))
		}
		b.WriteString("\n")
	}

	golden.RequireEqual(t, b.String())
}

// Line reads a path the way the trail shows it, then where it ends: the commands it runs, what it notes, nothing, or
// the question it is stuck at.
func Line(p Path) string {
	parts := make([]string, len(p.Answers))
	// Show every answer by how it shows
	for i, a := range p.Answers {
		parts[i] = Label(p.Asked[i], a.Values)
	}
	return strings.Join(parts, " / ") + " → " + End(p)
}

// End is where a path ends: the commands it runs, what it notes, nothing, or the question it is stuck at.
func End(p Path) string {
	// Name the question a stuck path ends at
	if p.Stuck {
		return "stuck at " + p.Asked[len(p.Asked)-1].Key
	}
	var ends []string
	// Name every command and note
	for _, c := range p.Plan {
		// Name a note by its words
		if len(c.Args) == 0 {
			ends = append(ends, "("+c.Note+")")
			continue
		}
		ends = append(ends, "o3s "+strings.Join(c.Args, " ")+marks(c))
	}
	// Name a path that runs nothing
	if len(ends) == 0 {
		return "back"
	}
	return strings.Join(ends, "; ")
}

// marks name how a command runs where it runs apart: quietly, on the real terminal, or restarting the menu.
func marks(c menu.Command) string {
	var out []string
	// Name every way the command runs apart
	for _, m := range []struct {
		on   bool
		name string
	}{{c.Quiet, "quiet"}, {c.Terminal, "terminal"}, {c.Restart, "restart"}} {
		// Name a way that applies
		if m.on {
			out = append(out, m.name)
		}
	}
	// Name nothing for a plain command
	if len(out) == 0 {
		return ""
	}
	return " [" + strings.Join(out, ", ") + "]"
}

// Label is how an answer shows: an option by its label, a view's pick by its tab and row, a hidden value hidden, and
// any other value as typed.
func Label(q menu.Question, values []string) string {
	// Show the answer by the kind of question
	switch q.Kind {
	case menu.Select, menu.Multi:
		return strings.Join(shown(q.Options, values), ", ")
	case menu.Hidden:
		return "***"
	case menu.Input, menu.Confirm:
		return strings.Join(values, ", ")
	case menu.View:
		i := slices.IndexFunc(q.Tabs, func(t menu.Tab) bool { return len(values) > 0 && t.Name == values[0] })
		// Show a pick on a tab the view lacks as typed
		if i < 0 {
			return strings.Join(values, " ")
		}
		t := q.Tabs[i]
		// Show a new order by its rows, keys and marks by their values
		switch {
		case len(values) > 1 && values[1] == menu.Order:
			return t.Name + " order " + strings.Join(shown(t.Options, values[3:]), ", ")
		case t.Multi:
			return t.Name + " [" + strings.Join(shown(t.Options, values[1:]), ", ") + "]"
		}
		return strings.TrimSpace(t.Name + " " + strings.Join(shown(append(slices.Clone(t.Options), keyOptions(t)...), values[1:]), ", "))
	}
	return strings.Join(values, ", ")
}

// keyOptions are a tab's keys as options, each by its help.
func keyOptions(t menu.Tab) []menu.Option {
	out := make([]menu.Option, len(t.Keys))
	// Take every key as an option
	for i, k := range t.Keys {
		out[i] = menu.Option{Value: k.Value, Label: k.Key + " " + k.Help}
	}
	return out
}

// shown are values by their options' labels, a value no option names as it is.
func shown(options []menu.Option, values []string) []string {
	out := make([]string, len(values))
	// Show every value by its option
	for i, v := range values {
		out[i] = v
		// Take the label of the option the value names
		if j := slices.IndexFunc(options, func(o menu.Option) bool { return o.Value == v }); j >= 0 {
			out[i] = options[j].Shown()
		}
	}
	return out
}
