package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// choice takes one of a question's options, filtered as the user types, and shows a detail's facts above its
// actions.
type choice struct {
	q menu.Question
	list
}

// newChoice starts a choice with the cursor on the question's default.
func newChoice(q menu.Question) choice {
	c := choice{q: q}
	c.options, c.numbered = q.Options, q.Numbered
	// Start on the default where the question names one
	if len(q.Default) > 0 {
		c.cursor = max(slices.IndexFunc(q.Options, func(o menu.Option) bool { return o.Value == q.Default[0] }), 0)
	}
	return c
}

// Update follows a key: a digit takes its option, typing filters, Enter takes the highlighted option, Esc clears the
// filter or steps back.
func (c choice) Update(msg tea.Msg) (field, result, tea.Cmd) {
	k, isKey := msg.(tea.KeyPressMsg)
	// Keep the field as it is on anything but a key
	if !isKey {
		return c, result{}, nil
	}
	// Take the option a digit numbers
	if o, ok := c.picked(k.Text); ok {
		return c, result{outcome: answered, values: []string{o.Value}}, nil
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
		c.move(k.String(), k.Text, c.filters())
	}
	return c, result{}, nil
}

// filters reports whether typing filters the options: everywhere but the numbered ones a digit picks and the actions
// of a detail.
func (c choice) filters() bool {
	return !c.numbered && len(c.q.Facts) == 0
}

// View draws the title, the facts of a detail, the filter once typed, and the options.
func (c choice) View(width, height int) string {
	lines := heading(c.q, width)
	// Show a detail's facts above what the user can do
	if len(c.q.Facts) > 0 {
		lines = append(lines, "")
		lines = append(lines, facts(c.q.Facts, width)...)
		lines = append(lines, "")
	}
	// Show the filter once the user types
	if c.filter != "" {
		lines = append(lines, dim.Render("  ⌕ ")+c.filter)
	}
	lines = append(lines, c.view(width, height-len(lines))...)
	return strings.Join(lines, "\n")
}

// Keys are the keys of a choice.
func (c choice) Keys() []key.Binding {
	keys := []key.Binding{keyNavigate}
	// Name the digits of numbered options, and typing to filter where it filters
	switch {
	case c.numbered:
		keys = append(keys, digits(min(len(c.shown()), 9)))
	case c.filters():
		keys = append(keys, keyType)
	}
	keys = append(keys, keySelect, keyBack)
	// Name Esc as clearing a filter the user typed
	if c.filter != "" {
		keys[len(keys)-1] = keyClear
	}
	return keys
}
