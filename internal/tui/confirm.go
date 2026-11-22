package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// yes and no are the agreement and the refusal a confirmation gives.
const (
	yes = "yes"
	no  = "no"
)

// confirm takes an agreement or a refusal, numbered one below the other, showing the change it is about above the
// question.
type confirm struct {
	q   menu.Question
	yes bool
}

// newConfirm starts a confirmation on its default, and on the refusal where the default is empty.
func newConfirm(q menu.Question) confirm {
	return confirm{q: q, yes: len(q.Default) > 0 && q.Default[0] == yes}
}

// Update follows a key: 1, 2, y and n answer at once, the arrows move between the answers, Enter takes the one
// chosen, Esc steps back.
func (c confirm) Update(msg tea.Msg) (field, result, tea.Cmd) {
	k, isKey := msg.(tea.KeyPressMsg)
	// Keep the field as it is on anything but a key
	if !isKey {
		return c, result{}, nil
	}
	// Follow the key the user pressed
	switch k.String() {
	case "esc":
		return c, result{outcome: back}, nil
	case "y", "1":
		return c, result{outcome: answered, values: []string{yes}}, nil
	case "n", "2":
		return c, result{outcome: answered, values: []string{no}}, nil
	case "up", "down":
		c.yes = !c.yes
	case "enter":
		// Answer with the choice the user moved to
		if c.yes {
			return c, result{outcome: answered, values: []string{yes}}, nil
		}
		return c, result{outcome: answered, values: []string{no}}, nil
	}
	return c, result{}, nil
}

// View draws the question and both answers numbered, the chosen one pointed at.
func (c confirm) View(width, _ int) string {
	lines := heading(c.q, width)
	yesLine, noLine := "  1. Yes", "  2. No"
	// Point at the answer the user is on
	if c.yes {
		yesLine = pointer.Render("❯ ") + "1. Yes"
	} else {
		noLine = pointer.Render("❯ ") + "2. No"
	}
	return strings.Join(append(lines, yesLine, noLine), "\n")
}

// Keys are the keys of a confirmation.
func (confirm) Keys() []key.Binding {
	return []key.Binding{keyNavigate, digits(2), keyYesNo, keySelect, keyBack}
}
