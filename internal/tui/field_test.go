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

	view := stripped(newMulti(q).View(usualWidth, 0))

	// Require the count of the options below
	if !strings.Contains(view, "↓ 3 more") {
		t.Errorf("view lacks ↓ 3 more:\n%s", view)
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
	_, r := press(f, keys(esc, "cl", space, enter)...)

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
	if view := stripped(f.View(narrow, 0)); !containsFold(view, "•••") {
		t.Errorf("hidden view = %q, want dots", view)
	}
}

// A confirmation answers on its numbers, y or n at once, and on Enter with the one the arrows chose, no by default.
func TestConfirmAnswersYesOrNo(t *testing.T) {
	apply := menu.Question{Key: "apply", Kind: menu.Confirm, Title: "Apply?"}

	_, plain := press(newConfirm(apply), enter)
	_, moved := press(newConfirm(apply), down, enter)
	_, typedYes := press(newConfirm(apply), keys("y")...)
	_, one := press(newConfirm(apply), keys("1")...)
	_, two := press(newConfirm(apply), keys("2")...)

	got := [][]string{plain.values, moved.values, typedYes.values, one.values, two.values}

	// Report answers that differ
	if diff := cmp.Diff([][]string{{no}, {yes}, {yes}, {yes}, {no}}, got); diff != "" {
		t.Errorf("answers (-want +got):\n%s", diff)
	}
	// Require both answers numbered, the one chosen pointed at
	if view := stripped(newConfirm(apply).View(usualWidth, 0)); !strings.Contains(view, "  1. Yes") || !strings.Contains(view, "❯ 2. No") {
		t.Errorf("view:\n%s", view)
	}
}

// A digit picks the option it numbers at once, where the options are numbered.
func TestChoicePicksANumberedOption(t *testing.T) {
	q := menu.Question{Key: "ports", Kind: menu.Select, Title: "Ports", Numbered: true, Options: []menu.Option{{Value: "443"}, {Value: "22,443"}}}

	_, r := press(newChoice(q), keys("2")...)

	// Require the second option taken
	if r.outcome != answered || !cmp.Equal([]string{"22,443"}, r.values) {
		t.Errorf("answer = %v, want 22,443", r)
	}
	// Require the options numbered
	if view := stripped(newChoice(q).View(usualWidth, 0)); !strings.Contains(view, "1. 443") || !strings.Contains(view, "2. 22,443") {
		t.Errorf("view:\n%s", view)
	}
}

// A detail shows its facts above its actions, which typing leaves as they are.
func TestChoiceShowsADetailsFacts(t *testing.T) {
	q := menu.Question{Key: "action", Kind: menu.Select, Title: fixture.Host, Facts: []menu.Fact{{Label: "Ports", Value: "443"}, {Label: "Needed by", Value: "user"}}, Options: []menu.Option{{Value: "remove", Label: "Remove"}, {Value: "+back", Label: "Back"}}}

	f, _ := press(newChoice(q), keys("zzz")...)

	view := stripped(f.View(usualWidth, 0))
	// Require the facts lined up and every action, unfiltered
	if !strings.Contains(view, "Ports       443") || !strings.Contains(view, "Needed by   user") || !strings.Contains(view, "Remove") || !strings.Contains(view, "Back") {
		t.Errorf("view:\n%s", view)
	}
}

// A text field reads the line as the user types, naming the rule it breaks, and holds Enter until it reads.
func TestTextChecksTheLineAsItIsTyped(t *testing.T) {
	q := menu.Question{Key: "hosts", Kind: menu.Input, Title: "Hosts", Example: fixture.Host, Check: func(s string) (string, error) {
		// Turn away a line with a space in a name
		if strings.Contains(s, "!") {
			return "", fmt.Errorf("%q is not a host name", s)
		}
		return "1 host", nil
	}}

	bad, held := press(newText(q), keys("bad!", enter)...)
	good, taken := press(newText(q), keys(fixture.Host, enter)...)

	// Require the bad line held with its rule named, and the good one taken with its reading
	if held.outcome != open || !strings.Contains(stripped(bad.View(usualWidth, 0)), "✗ \"bad!\" is not a host name") {
		t.Errorf("bad line = %v, view:\n%s", held, stripped(bad.View(usualWidth, 0)))
	}
	// Require the good line taken
	if taken.outcome != answered || !cmp.Equal([]string{fixture.Host}, taken.values) {
		t.Errorf("good line = %v, want %s taken", taken, fixture.Host)
	}
	// Require the example shown and the reading named
	if view := stripped(good.View(usualWidth, 0)); !strings.Contains(view, "e.g. "+fixture.Host) {
		t.Errorf("view lacks the example:\n%s", view)
	}
	reading, _ := press(newText(q), keys(fixture.Host)...)
	// Require the reading named while the line holds
	if view := stripped(reading.View(usualWidth, 0)); !strings.Contains(view, "✓ 1 host") {
		t.Errorf("view lacks the reading:\n%s", view)
	}
}

// aView is a view of two tabs: hosts the user picks from with a key that acts, and names the user marks.
var aView = menu.Question{Key: "view", Kind: menu.View, Title: "Firewall", Tabs: []menu.Tab{
	{Name: "Allowed", About: "The cage reaches only these hosts.", Options: []menu.Option{{Value: "+add", Label: "+ Allow a host…"}, {Value: fixture.Host, Columns: []string{"443"}}, {Value: fixture.OtherHost, Columns: []string{"22,443"}}}, Keys: []menu.Key{{Key: "p", Help: "prune expired", Value: "+prune"}}},
	{Name: "Blocked", Multi: true, Options: []menu.Option{{Value: fixture.RefusedName}, {Value: fixture.AbsentName}}, Marked: []string{fixture.AbsentName}, Empty: "Nothing blocked yet."},
}}

// A view picks a row on its tab, switches tabs both ways around, hands back the marks of a tab that takes them, and
// answers with a tab's own key.
func TestViewPicksSwitchesMarksAndActs(t *testing.T) {
	_, picked := press(newView(aView), down, enter)
	_, marked := press(newView(aView), right, space, enter)
	_, around := press(newView(aView), right, right, enter)
	_, acted := press(newView(aView), keys("p")...)

	got := [][]string{picked.values, marked.values, around.values, acted.values}

	// Report answers that differ
	if diff := cmp.Diff([][]string{{"Allowed", fixture.Host}, {"Blocked", fixture.RefusedName, fixture.AbsentName}, {"Allowed", "+add"}, {"Allowed", "+prune"}}, got); diff != "" {
		t.Errorf("answers (-want +got):\n%s", diff)
	}
}

// A view searches once / opens its box, Esc clearing the search before it steps back.
func TestViewSearchesAfterTheSlash(t *testing.T) {
	f, _ := press(newView(aView), keys("/", "org")...)
	_, r := press(f, enter, enter)
	_, cleared := press(f, esc)
	_, stepped := press(f, esc, esc)

	// Require the search to keep the matching row and Enter to take it
	if !cmp.Equal([]string{"Allowed", fixture.OtherHost}, r.values) {
		t.Errorf("after the search = %v, want %s", r.values, fixture.OtherHost)
	}
	// Require Esc to clear the search first, then step back
	if cleared.outcome != open || stepped.outcome != back {
		t.Errorf("esc = %v then %v, want open then back", cleared.outcome, stepped.outcome)
	}
	// Require the search box with what the user typed
	if view := stripped(f.View(usualWidth, usualHeight)); !strings.Contains(view, "⌕ org") {
		t.Errorf("view:\n%s", view)
	}
}

// A view's tab that lets rows move answers with the row and the direction on ctrl+arrows.
func TestViewMovesARow(t *testing.T) {
	q := aView
	q.Tabs = []menu.Tab{{Name: "Installed", Reorder: true, Options: []menu.Option{{Value: fixture.Feature}, {Value: fixture.OtherFeature}}}}

	_, r := press(newView(q), tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})

	// Require the row and the direction
	if !cmp.Equal([]string{"Installed", "+move", fixture.Feature, "down"}, r.values) {
		t.Errorf("answer = %v, want the feature moved down", r.values)
	}
}

// A view shows its tabs, what the tab holds, the rows with their columns lined up, and what an empty tab says.
func TestViewShowsTheTabsAndTheRows(t *testing.T) {
	f := newView(aView)
	empty := aView
	empty.Tabs = []menu.Tab{aView.Tabs[0], {Name: "Blocked", Empty: "Nothing blocked yet."}}
	e, _ := press(newView(empty), right)

	view := stripped(f.View(usualWidth, usualHeight))
	// Require the tab bar, the about line, the search box and the rows lined up
	for _, want := range []string{"Firewall", "Allowed", "Blocked", "The cage reaches only these hosts.", "⌕ Search…", "❯ + Allow a host…"} {
		// Report a part the view lacks
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	// Require the columns lined up across the rows
	if host, other := strings.Index(view, fixture.Host), strings.Index(view, fixture.OtherHost); strings.Index(view[host:], "443") != strings.Index(view[other:], "22,443") {
		t.Errorf("columns differ in where they start:\n%s", view)
	}
	// Require what an empty tab says
	if view := stripped(e.View(usualWidth, usualHeight)); !strings.Contains(view, "Nothing blocked yet.") {
		t.Errorf("empty tab:\n%s", view)
	}
}

// A row longer than the screen ends in an ellipsis within the width.
func TestListCutsALongRow(t *testing.T) {
	q := menu.Question{Key: "feature", Kind: menu.Select, Title: "Features", Options: []menu.Option{{Value: "tool", Detail: strings.Repeat("does a lot ", 20)}}}

	view := stripped(newChoice(q).View(narrow, 0))

	// Require every line within the width, the long one cut
	for line := range strings.SplitSeq(view, "\n") {
		// Report a line wider than the screen
		if len([]rune(line)) > narrow {
			t.Errorf("line %q wider than %d", line, narrow)
		}
	}
	// Require the cut named with an ellipsis
	if !strings.Contains(view, "…") {
		t.Errorf("view lacks the ellipsis:\n%s", view)
	}
}
