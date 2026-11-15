package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// choice takes one of a question's options, filtered as the user types.
type choice struct {
	list
}

// newChoice starts a choice with the cursor on the question's default.
func newChoice(q menu.Question) choice {
	c := choice{list{q: q}}
	// Start on the default where the question names one
	if len(q.Default) > 0 {
		c.cursor = max(slices.IndexFunc(q.Options, func(o menu.Option) bool { return o.Value == q.Default[0] }), 0)
	}
	return c
}

// Update follows a key: typing filters, Enter takes the highlighted option, Esc clears the filter or steps back.
func (c choice) Update(msg tea.Msg) (field, result, tea.Cmd) {
	k, isKey := msg.(tea.KeyPressMsg)
	// Keep the field as it is on anything but a key
	if !isKey {
		return c, result{}, nil
	}
	// Follow the key the user pressed
	switch k.String() {
	case "esc":
		// Step back where nothing is filtered, else clear the filter
		if c.filter == "" {
			return c, result{outcome: back}, nil
		}
		c.filter, c.cursor = "", 0
	case "enter":
		// Take the highlighted option where the filter keeps one
		if o, ok := c.highlighted(); ok {
			return c, result{outcome: answered, values: []string{o.Value}}, nil
		}
	default:
		c.move(k.String(), k.Text)
	}
	return c, result{}, nil
}

// View draws the options with a pointer.
func (c choice) View(int) string {
	return c.view(func(menu.Option) string { return "" })
}

// Hint names the keys of a choice.
func (choice) Hint() string {
	return "type to filter | up/down move | enter choose | esc back"
}
