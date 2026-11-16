package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// yes and no are the answers a confirmation gives.
const (
	yes = "yes"
	no  = "no"
)

// confirm takes yes or no, showing the change it is about above the question.
type confirm struct {
	q   menu.Question
	yes bool
}

// newConfirm starts a confirmation on its default, and on no where the question names none.
func newConfirm(q menu.Question) confirm {
	return confirm{q: q, yes: len(q.Default) > 0 && q.Default[0] == yes}
}

// Update follows a key: y and n answer at once, the arrows move between them, Enter takes the one chosen, Esc steps back.
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
	case "y":
		return c, result{outcome: answered, values: []string{yes}}, nil
	case "n":
		return c, result{outcome: answered, values: []string{no}}, nil
	case "left", "right", "tab", "shift+tab":
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

// View draws the question and both answers with the chosen one highlighted.
func (c confirm) View(int) string {
	var lines []string
	yesLabel, noLabel := "  Yes  ", "  No  "
	// Highlight the answer the user is on
	if c.yes {
		yesLabel = pointer.Render("> Yes  ")
	} else {
		noLabel = pointer.Render("> No  ")
	}
	lines = append(lines, title.Render(c.q.Title), yesLabel+noLabel)
	return strings.Join(lines, "\n")
}

// Hint names the keys of a confirmation.
func (confirm) Hint() string {
	return "y/n | left/right move | enter confirm | esc back"
}
