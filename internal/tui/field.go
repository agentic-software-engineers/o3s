package tui

import (
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
	// View draws the question in the width given.
	View(width int) string
	// Hint names the keys the field takes.
	Hint() string
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
	case menu.Select:
		return newChoice(q)
	}
	return newChoice(q)
}

// matching lists the options that contain a filter, in the question's order.
func matching(options []menu.Option, filter string) []menu.Option {
	var out []menu.Option
	// Keep every option that matches
	for _, o := range options {
		// Take an option that contains the filter
		if containsFold(o.Value, filter) {
			out = append(out, o)
		}
	}
	return out
}
