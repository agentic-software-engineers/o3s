package tui

import tea "charm.land/bubbletea/v2"

// Keys the tests press.
var (
	space = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	right = tea.KeyPressMsg{Code: tea.KeyRight}
	ctrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// typed is the key presses of a text, one per character.
func typed(s string) []tea.Msg {
	var out []tea.Msg
	// Press every character in turn
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

// keys joins key presses and typed texts into one sequence.
func keys(parts ...any) []tea.Msg {
	var out []tea.Msg
	// Take every part as a key or a typed text
	for _, p := range parts {
		// Type a text, or press a key
		if s, isText := p.(string); isText {
			out = append(out, typed(s)...)
		} else {
			out = append(out, p.(tea.Msg))
		}
	}
	return out
}
