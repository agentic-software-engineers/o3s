package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// text takes a line, shown or hidden, starting from the question's default where it is shown.
type text struct {
	q     menu.Question
	input textinput.Model
}

// newText starts a text field with a steady cursor, holding the default of a shown question.
func newText(q menu.Question) text {
	in := textinput.New()
	st := in.Styles()
	st.Cursor.Blink = false
	in.SetStyles(st)
	in.Prompt = "> "
	// Hide what the user types for a secret
	if q.Kind == menu.Hidden {
		in.EchoMode = textinput.EchoPassword
		in.EchoCharacter = '•'
	}
	// Start from the default of a shown question, so Enter keeps it
	if q.Kind == menu.Input && len(q.Default) > 0 {
		in.SetValue(q.Default[0])
		in.CursorEnd()
	}
	in.Focus()
	return text{q: q, input: in}
}

// Update follows a key: Enter hands back the line, Esc steps back, and every other key edits the line.
func (t text) Update(msg tea.Msg) (field, result, tea.Cmd) {
	// Follow the keys that end the field
	if k, isKey := msg.(tea.KeyPressMsg); isKey {
		// Follow the key the user pressed
		switch k.String() {
		case "esc":
			return t, result{outcome: back}, nil
		case "enter":
			return t, result{outcome: answered, values: []string{strings.TrimSpace(t.input.Value())}}, nil
		}
	}
	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	return t, result{}, cmd
}

// View draws the title and the line.
func (t text) View(width int) string {
	t.input.SetWidth(max(width-4, 10))
	return title.Render(t.q.Title) + "\n" + t.input.View()
}

// Hint names the keys of a text field.
func (t text) Hint() string {
	// Name the empty answer a secret's value may take
	if t.q.Kind == menu.Hidden {
		return "hidden as you type · enter confirm · esc back"
	}
	return "enter confirm · esc back"
}
