package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// outcome is what a key did to a field.
type outcome int

const (
	// open keeps the field asking.
	open outcome = iota
	// answered ends the field with values.
	answered
	// back steps back to the previous question.
	back
)

// result is the outcome of a key and the values an answer carries.
type result struct {
	outcome outcome
	values  []string
}

// field asks one question on the screen the app draws.
type field interface {
	// Update follows a message and reports what it did.
	Update(msg tea.Msg) (field, result, tea.Cmd)
	// View draws the question within the width and the height given.
	View(width, height int) string
	// Keys are the keys the field takes now, as the footer names them.
	Keys() []key.Binding
}

// newField builds the field that asks a question of its kind.
func newField(q menu.Question) field {
	// Build the field of the question's kind
	switch q.Kind {
	case menu.Multi:
		return newMulti(q)
	case menu.Input, menu.Hidden:
		return newText(q)
	case menu.Confirm:
		return newConfirm(q)
	case menu.View:
		return newView(q)
	case menu.Select:
		return newChoice(q)
	}
	return newChoice(q)
}

// heading draws a question's title and the line that says what it is about, broken to the width.
func heading(q menu.Question, width int) []string {
	lines := []string{title.Render(q.Title)}
	// Say what the question is about
	if q.About != "" {
		lines = append(lines, dim.Render(strings.Join(wrapped(q.About, width), "\n")))
	}
	return lines
}

// facts draws labeled facts with their values lined up.
func facts(fs []menu.Fact, width int) []string {
	label := 0
	// Measure the longest label
	for _, f := range fs {
		label = max(label, len(f.Label))
	}
	var lines []string
	// Draw every fact
	for _, f := range fs {
		lines = append(lines, cut("  "+dim.Render(pad(f.Label, label))+"   "+f.Value, width))
	}
	return lines
}
