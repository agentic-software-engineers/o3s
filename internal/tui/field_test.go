package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/menu"
)

// press sends keys to a field and hands back where it ends and the last outcome.
func press(f field, msgs ...tea.Msg) (field, result) {
	var r result
	// Send every key in turn
	for _, msg := range msgs {
		f, r, _ = f.Update(msg)
	}
	return f, r
}

// features is a store's list as the feature question offers it.
var features = menu.Question{Key: "feature", Kind: menu.Multi, Title: "Features", Options: []menu.Option{{Value: "claude-code"}, {Value: "codex"}, {Value: "go"}, {Value: "golangci-lint"}, {Value: "rust"}}}

// groups is a choice of groups with notes.
var groups = menu.Question{Key: "do", Kind: menu.Select, Title: "What do you want to do?", Options: []menu.Option{{Value: "firewall", Note: "allow hosts"}, {Value: "secret", Note: "values"}, {Value: "store", Note: "stores"}}}

// A long list names how many options wait below its window.
func TestListNamesTheOptionsBelow(t *testing.T) {
	q := menu.Question{Key: "feature", Kind: menu.Multi, Title: "Features"}
	// Offer more options than the window holds
	for i := range rows + 3 {
		q.Options = append(q.Options, menu.Option{Value: fmt.Sprintf("feature-%02d", i)})
	}

	view := stripped(newMulti(q).View(usualWidth))

	// Require the count of the options below
	if !strings.Contains(view, "(3 more)") {
		t.Errorf("view lacks (3 more):\n%s", view)
	}
}

// A choice filters as the user types, takes the highlighted option on Enter, and starts on its default.
func TestChoiceFiltersAndTakesTheHighlightedOption(t *testing.T) {
	_, filtered := press(newChoice(groups), keys("st", enter)...)
	q := groups
	q.Default = []string{"secret"}
	_, defaulted := press(newChoice(q), enter)

	got := [][]string{filtered.values, defaulted.values}

	// Report answers that differ
	if diff := cmp.Diff([][]string{{"store"}, {"secret"}}, got); diff != "" {
		t.Errorf("answers (-want +got):\n%s", diff)
	}
}

// Esc clears a filter first, and steps back on an empty one, in every list.
func TestListsClearTheFilterBeforeTheyStepBack(t *testing.T) {
	// Check the choice and the multi-select alike
	for _, f := range []field{newChoice(groups), newMulti(features)} {
		_, cleared := press(f, keys("s", esc)...)
		_, stepped := press(f, keys("s", esc, esc)...)

		// Require the filter cleared without an outcome, then a step back
		if cleared.outcome != open || stepped.outcome != back {
			t.Errorf("%T: esc = %v then %v, want open then back", f, cleared.outcome, stepped.outcome)
		}
	}
}

// A multi-select marks while filtering, keeps the filter after a mark, and hands back the marks in order.
func TestMultiMarksWhileFiltering(t *testing.T) {
	f, _ := press(newMulti(features), keys("ru", space)...)
	_, r := press(f, keys(esc, "cl", tab, enter)...)

	// Require the filter kept after a mark
	if m := f.(multi); m.filter != "ru" {
		t.Errorf("filter = %q, want ru", m.filter)
	}
	// Require the marks in the store's order
	if diff := cmp.Diff([]string{"claude-code", "rust"}, r.values); diff != "" || r.outcome != answered {
		t.Errorf("answer (-want +got):\n%s", diff)
	}
}

// Enter without a mark takes the highlighted option, and the defaults start marked.
func TestMultiTakesTheHighlightedOptionOrTheDefaults(t *testing.T) {
	_, highlighted := press(newMulti(features), down, down, enter)
	q := features
	q.Default = []string{"go"}
	_, defaulted := press(newMulti(q), enter)

	got := [][]string{highlighted.values, defaulted.values}

	// Report answers that differ
	if diff := cmp.Diff([][]string{{"go"}, {"go"}}, got); diff != "" {
		t.Errorf("answers (-want +got):\n%s", diff)
	}
}

// A shown text starts from its default, and a hidden one starts empty and shows dots.
func TestTextStartsFromItsDefault(t *testing.T) {
	ports := menu.Question{Key: "ports", Kind: menu.Input, Title: "Ports", Default: []string{strconv.Itoa(fixture.Port)}}
	value := menu.Question{Key: "value", Kind: menu.Hidden, Title: "Value", Default: []string{fixture.SecretValue}}

	_, kept := press(newText(ports), enter)
	f, typedValue := press(newText(value), keys("abc", enter)...)

	// Require the default kept, and the hidden value typed alone
	if diff := cmp.Diff([][]string{{strconv.Itoa(fixture.Port)}, {"abc"}}, [][]string{kept.values, typedValue.values}); diff != "" {
		t.Errorf("answers (-want +got):\n%s", diff)
	}
	// Require the hidden value shown as dots
	if view := stripped(f.View(narrow)); !containsFold(view, "***") {
		t.Errorf("hidden view = %q, want dots", view)
	}
}

// A confirmation answers on y or n at once, and on Enter with the one the arrows chose, no by default.
func TestConfirmAnswersYesOrNo(t *testing.T) {
	apply := menu.Question{Key: "apply", Kind: menu.Confirm, Title: "Apply?"}

	_, plain := press(newConfirm(apply), enter)
	_, moved := press(newConfirm(apply), right, enter)
	_, typedYes := press(newConfirm(apply), keys("y")...)

	got := [][]string{plain.values, moved.values, typedYes.values}

	// Report answers that differ
	if diff := cmp.Diff([][]string{{no}, {yes}, {yes}}, got); diff != "" {
		t.Errorf("answers (-want +got):\n%s", diff)
	}
}
