package tui

import (
	"maps"

	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// multi takes any number of a question's options, filtered as the user types and marked with Space or Tab.
type multi struct {
	list
	marks map[string]bool
}

// newMulti starts a multi-select with the question's defaults marked.
func newMulti(q menu.Question) multi {
	m := multi{q: q, marks: map[string]bool{}}
	// Mark every default
	for _, d := range q.Default {
		m.marks[d] = true
	}
	return m
}

// Update follows a key: typing filters, Space and Tab mark, Enter hands back, Esc clears the filter or steps back.
func (m multi) Update(msg tea.Msg) (field, result, tea.Cmd) {
	k, isKey := msg.(tea.KeyPressMsg)
	// Keep the field as it is on anything but a key
	if !isKey {
		return m, result{}, nil
	}
	// Follow the key the user pressed
	switch k.String() {
	case "esc":
		// Step back where nothing is filtered, else clear the filter
		if m.filter == "" {
			return m, result{outcome: back}, nil
		}
		m.filter, m.cursor = "", 0
	case "enter":
		// Hand back the marks, or the highlighted option where none is marked
		if values := m.chosen(); len(values) > 0 {
			return m, result{outcome: answered, values: values}, nil
		}
	case "space", "tab":
		// Mark or unmark the highlighted option
		if o, ok := m.highlighted(); ok {
			m.marks = toggled(m.marks, o.Value)
		}
	default:
		m.move(k.String(), k.Text)
	}
	return m, result{}, nil
}

// chosen hands back the marked options in the question's order, or the highlighted one where none is marked.
func (m multi) chosen() []string {
	var out []string
	// Take every marked option in order
	for _, o := range m.q.Options {
		// Take an option the user marked
		if m.marks[o.Value] {
			out = append(out, o.Value)
		}
	}
	// Take the highlighted option where the user marked none
	if o, ok := m.highlighted(); len(out) == 0 && ok {
		out = []string{o.Value}
	}
	return out
}

// View draws the options with their marks.
func (m multi) View(int) string {
	return m.view(func(o menu.Option) string {
		// Fill the box of a marked option
		if m.marks[o.Value] {
			return marked.Render("[•]") + " "
		}
		return "[ ] "
	})
}

// Hint names the keys of a multi-select.
func (multi) Hint() string {
	return "type to filter · space mark · enter done · esc back"
}

// toggled hands back marks with one value flipped, leaving the marks given as they were.
func toggled(marks map[string]bool, value string) map[string]bool {
	out := make(map[string]bool, len(marks)+1)
	maps.Copy(out, marks)
	out[value] = !out[value]
	return out
}
