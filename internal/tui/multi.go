package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// multi takes any number of a question's options, filtered as the user types and marked with Space.
type multi struct {
	q menu.Question
	list
}

// newMulti starts a multi-select with the question's defaults marked.
func newMulti(q menu.Question) multi {
	m := multi{q: q}
	m.options, m.marks = q.Options, map[string]bool{}
	// Mark every default
	for _, d := range q.Default {
		m.marks[d] = true
	}
	return m
}

// Update follows a key: typing filters, Space marks, Enter hands back the marks, Esc clears the filter or steps back.
func (m multi) Update(msg tea.Msg) (field, result, tea.Cmd) {
	k, isKey := msg.(tea.KeyPressMsg)
	// Keep the field as it is on anything but a key
	if !isKey {
		return m, result{}, nil
	}
	// Follow the key the user pressed
	switch k.String() {
	case "esc":
		// Step back where the filter is empty, else clear the filter
		if m.filter == "" {
			return m, result{outcome: back}, nil
		}
		m.filter, m.cursor = "", 0
	case "enter":
		values := m.marked()
		// Take the highlighted option where the marks are empty
		if o, ok := m.highlighted(); len(values) == 0 && ok {
			values = []string{o.Value}
		}
		// Hand back what the user chose
		if len(values) > 0 {
			return m, result{outcome: answered, values: values}, nil
		}
	case "space":
		m.toggle()
	default:
		m.move(k.String(), k.Text, m.q.Filterable)
	}
	return m, result{}, nil
}

// View draws the title, the filter once typed, and the options with their marks.
func (m multi) View(width, height int) string {
	lines := heading(m.q, width)
	// Show the filter once the user types
	if m.filter != "" {
		lines = append(lines, dim.Render("  ⌕ ")+m.filter)
	}
	lines = append(lines, m.view(width, height-len(lines))...)
	return strings.Join(lines, "\n")
}

// Keys are the keys of a multi-select.
func (m multi) Keys() []key.Binding {
	esc := keyBack
	// Name Esc as clearing a filter the user typed
	if m.filter != "" {
		esc = keyClear
	}
	// Name typing to search where the list takes it
	if m.q.Filterable {
		return []key.Binding{keyNavigate, keyToggle, keyType, keyContinue, esc}
	}
	return []key.Binding{keyNavigate, keyToggle, keyContinue, esc}
}
