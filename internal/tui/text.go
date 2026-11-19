package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// text takes a line, shown or hidden, starting from the question's default where it is shown, and reads it as the
// user types where the question checks it.
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
	in.Prompt = "❯ "
	// Hide what the user types for a secret
	if q.Kind == menu.Hidden {
		in.EchoMode = textinput.EchoPassword
		in.EchoCharacter = '•'
	}
	// Complete the line to the values the question suggests
	if len(q.Suggestions) > 0 {
		in.ShowSuggestions = true
		in.SetSuggestions(q.Suggestions)
	}
	// Start from the default of a shown question, so Enter keeps it
	if q.Kind == menu.Input && len(q.Default) > 0 {
		in.SetValue(q.Default[0])
		in.CursorEnd()
	}
	in.Focus()
	return text{q: q, input: in}
}

// reading is how the question reads the line so far, and the rule it breaks where it breaks one.
func (t text) reading() (string, error) {
	// Read nothing where the question checks nothing or nothing is typed yet
	if t.q.Check == nil || strings.TrimSpace(t.input.Value()) == "" {
		return "", nil
	}
	return t.q.Check(t.input.Value())
}

// Update follows a key: Enter hands back a line the question takes, Esc steps back, and every other key edits the
// line.
func (t text) Update(msg tea.Msg) (field, result, tea.Cmd) {
	// Follow the keys that end the field
	if k, isKey := msg.(tea.KeyPressMsg); isKey {
		// Follow the key the user pressed
		switch k.String() {
		case "esc":
			return t, result{outcome: back}, nil
		case "enter":
			_, err := t.reading()
			// Hold a line the question turns away, or an empty one it checks
			if err != nil || t.q.Check != nil && strings.TrimSpace(t.input.Value()) == "" {
				return t, result{}, nil
			}
			return t, result{outcome: answered, values: []string{strings.TrimSpace(t.input.Value())}}, nil
		}
	}
	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	return t, result{}, cmd
}

// View draws the title, what the line is about, an example of its form, the line, and how the question reads it.
func (t text) View(width, _ int) string {
	lines := heading(t.q, width)
	// Show the form the line takes
	if t.q.Example != "" {
		lines = append(lines, dim.Render(cut("e.g. "+t.q.Example, width)))
	}
	t.input.SetWidth(max(width-fieldMargin, minFieldWidth))
	lines = append(lines, t.input.View())
	reading, err := t.reading()
	// Say how the question reads the line, or the rule it breaks
	switch {
	case err != nil:
		lines = append(lines, warning.Render(cut("✗ "+err.Error(), width)))
	case reading != "":
		lines = append(lines, added.Render(cut("✓ "+reading, width)))
	}
	return strings.Join(lines, "\n")
}

// Keys are the keys of a text field, with completing where the question suggests values.
func (t text) Keys() []key.Binding {
	// Name completing where the question suggests values
	if len(t.q.Suggestions) > 0 {
		return []key.Binding{keyComplete, keyContinue, keyCancel}
	}
	return []key.Binding{keyContinue, keyCancel}
}
