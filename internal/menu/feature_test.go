package menu

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
)

// The fake store's features by their packages, and the lines its previews show for the hosts they bring or take.
var (
	toolPkg       = fixture.Store + "/" + fixture.Feature
	otherPkg      = fixture.Store + "/" + fixture.OtherFeature
	hostLine      = fmt.Sprintf("  + %s:%d  %s !", fixture.Host, fixture.Port, fixture.Secret)
	otherHostLine = fmt.Sprintf("  + %s:%d  %s !", fixture.OtherHost, fixture.Port, fixture.OtherSecret)
	goneLine      = fmt.Sprintf("  - %s:%d", fixture.Host, fixture.Port)
)

// fakeLookup is a store that offers the tool, whose feature needs two secrets still waiting for values.
type fakeLookup struct{}

// Features lists the ids the store offers, and fails for any other store.
func (fakeLookup) Features(store string) ([]string, error) {
	// Turn away a store out of reach
	if store != fixture.Store {
		return nil, errors.New("read the features of " + store + ": unreachable")
	}
	return []string{fixture.OtherFeature, fixture.Feature}, nil
}

// Add shows what a feature brings, the tool with the options it offers.
func (fakeLookup) Add(id string) (Preview, error) {
	pv := Preview{Lines: []string{id + ":" + fixture.Major, hostLine, otherHostLine}, Waiting: []string{fixture.Secret, fixture.OtherSecret}, Features: 1, Hosts: 2}
	// Offer the tool's options alone
	if id == toolPkg {
		pv.Options = []FeatureOption{
			{Name: "debug", Type: "boolean", Default: "false", Description: "Log every step"},
			{Name: "mode", Type: "string", Default: "strict", Enum: []string{"strict", "loose"}},
			{Name: "version", Type: "string", Default: fixture.FloatingTag, Proposals: []string{fixture.FloatingTag, "lts"}},
		}
	}
	return pv, nil
}

// Remove shows what leaves with the tool.
func (fakeLookup) Remove(pkg string) (Preview, error) {
	return Preview{Lines: []string{pkg, goneLine}, Features: 1, Hosts: 1}, nil
}

// aHomeWithTheTool is aHome holding the tool and reading its store through the fake.
func aHomeWithTheTool() Home {
	h := aHome
	h.Features = []string{toolPkg}
	h.Lookup = fakeLookup{}
	return h
}

// The feature dialogs preview their change, ask for waiting values, and run the commands they lead to.
func TestFeatureDialogsEndInTheirCommands(t *testing.T) {
	value := fixture.SecretValue
	tests := []struct {
		name    string
		answers [][]string
		titles  []string
		want    []Command
	}{
		{
			"adding a feature with one value given and one skipped, its options kept",
			[][]string{{"feature"}, {"add"}, {fixture.Store}, {fixture.Feature}, {"yes"}, {"keep defaults"}, {value}, {""}},
			[]string{"What do you want to do?", "Action", "Store", "Features", "Apply?", "Options", "Value for " + fixture.Secret + " (empty to skip)", "Value for " + fixture.OtherSecret + " (empty to skip)"},
			[]Command{
				{Args: []string{"secret", "add", fixture.Secret, "--no-bind-hint"}, Stdin: &value},
				{Args: []string{"feature", "add", toolPkg, "--no-preview"}},
			},
		},
		{
			"configuring every option a feature offers, each written as answered",
			[][]string{{"feature"}, {"add"}, {fixture.Store}, {fixture.Feature}, {"yes"}, {"configure"}, {"true"}, {"loose"}, {"lts"}, {""}, {""}},
			[]string{"What do you want to do?", "Action", "Store", "Features", "Apply?", "Options", fixture.Feature + " debug", fixture.Feature + " mode", fixture.Feature + " version", "Value for " + fixture.Secret + " (empty to skip)", "Value for " + fixture.OtherSecret + " (empty to skip)"},
			[]Command{{Args: []string{"feature", "add", toolPkg, "--option", toolPkg + ":debug=true", "--option", toolPkg + ":mode=loose", "--option", toolPkg + ":version=lts", "--no-preview"}}},
		},
		{
			"declining a feature",
			[][]string{{"feature"}, {"add"}, {fixture.Store}, {fixture.Feature}, {"no"}},
			[]string{"What do you want to do?", "Action", "Store", "Features", "Apply?"},
			nil,
		},
		{
			"adding two features that wait for the same secrets",
			[][]string{{"feature"}, {"add"}, {fixture.Store}, {fixture.OtherFeature, fixture.Feature}, {"yes"}, {"keep defaults"}, {value}, {""}},
			[]string{"What do you want to do?", "Action", "Store", "Features", "Apply?", "Options", "Value for " + fixture.Secret + " (empty to skip)", "Value for " + fixture.OtherSecret + " (empty to skip)"},
			[]Command{
				{Args: []string{"secret", "add", fixture.Secret, "--no-bind-hint"}, Stdin: &value},
				{Args: []string{"feature", "add", otherPkg, toolPkg, "--no-preview"}},
			},
		},
		{
			"choosing no feature",
			[][]string{{"feature"}, {"add"}, {fixture.Store}, {}},
			[]string{"What do you want to do?", "Action", "Store", "Features"},
			[]Command{{Note: "No feature chosen"}},
		},
		{
			"a store out of reach",
			[][]string{{"feature"}, {"add"}, {fixture.AbsentStore}},
			[]string{"What do you want to do?", "Action", "Store"},
			[]Command{{Note: "error: read the features of " + fixture.AbsentStore + ": unreachable"}},
		},
		{
			"removing a feature",
			[][]string{{"feature"}, {"rm"}, {toolPkg}, {"yes"}},
			[]string{"What do you want to do?", "Action", "Feature", "Apply?"},
			[]Command{{Args: []string{"feature", "rm", toolPkg, "--no-preview"}}},
		},
		{
			"listing the features",
			[][]string{{"feature"}, {"ls"}},
			[]string{"What do you want to do?", "Action"},
			[]Command{{Args: []string{"feature", "ls"}}},
		},
	}
	h := aHomeWithTheTool()
	h.Stores = append(h.Stores, fixture.AbsentStore)
	// Walk every dialog
	for _, tt := range tests {
		asked, got := walk(t, h, tt.answers...)

		// Report questions that differ
		if diff := cmp.Diff(tt.titles, titles(asked)); diff != "" {
			t.Errorf("%s: questions (-want +got):\n%s", tt.name, diff)
		}
		// Report commands that differ
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("%s: commands (-want +got):\n%s", tt.name, diff)
		}
	}
}

// Each option asks in the shape of the value it takes, starting from its default.
func TestFeatureOptionsAskByTheirKind(t *testing.T) {
	asked, _ := walk(t, aHomeWithTheTool(), []string{"feature"}, []string{"add"}, []string{fixture.Store}, []string{fixture.Feature}, []string{"yes"}, []string{"configure"}, []string{"false"}, []string{"strict"}, []string{fixture.FloatingTag}, []string{""}, []string{""})

	got := asked[6:9]

	want := []Question{
		{Key: "option:" + toolPkg + ":debug", Kind: Select, Title: fixture.Feature + " debug", Options: []Option{{Value: "true"}, {Value: "false"}}, Default: []string{"false"}, Preview: []string{"Log every step"}},
		{Key: "option:" + toolPkg + ":mode", Kind: Select, Title: fixture.Feature + " mode", Options: []Option{{Value: "strict"}, {Value: "loose"}}, Default: []string{"strict"}},
		{Key: "option:" + toolPkg + ":version", Kind: Input, Title: fixture.Feature + " version", Default: []string{fixture.FloatingTag}, Preview: []string{"suggested: " + fixture.FloatingTag + ", lts"}},
	}
	// Report questions that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("option questions (-want +got):\n%s", diff)
	}
}

// Apply? shows what the feature brings or takes, as the CLI prints it.
func TestFeatureApplyShowsThePreview(t *testing.T) {
	h := aHomeWithTheTool()
	added, _ := walk(t, h, []string{"feature"}, []string{"add"}, []string{fixture.Store}, []string{fixture.Feature}, []string{"no"})
	removed, _ := walk(t, h, []string{"feature"}, []string{"rm"}, []string{toolPkg}, []string{"no"})

	got := [][]string{added[4].Preview, removed[3].Preview}

	want := [][]string{
		{toolPkg + ":" + fixture.Major, hostLine, otherHostLine},
		{toolPkg, goneLine},
	}
	// Report previews that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("previews (-want +got):\n%s", diff)
	}
}

// Apply? names what the change comes to in one line, which stays beside the question however long the list above it grows.
func TestFeatureApplySummarizesTheChange(t *testing.T) {
	h := aHomeWithTheTool()
	one, _ := walk(t, h, []string{"feature"}, []string{"add"}, []string{fixture.Store}, []string{fixture.Feature}, []string{"no"})
	two, _ := walk(t, h, []string{"feature"}, []string{"add"}, []string{fixture.Store}, []string{fixture.OtherFeature, fixture.Feature}, []string{"no"})
	removed, _ := walk(t, h, []string{"feature"}, []string{"rm"}, []string{toolPkg}, []string{"no"})

	got := []string{one[4].Summary, two[4].Summary, removed[3].Summary}

	want := []string{
		"Adds " + fixture.Feature + " with 2 hosts, 2 secrets wait for a value",
		"Adds 2 features with 4 hosts, 2 secrets wait for a value",
		"Removes " + fixture.Feature + " with 1 host",
	}
	// Report summaries that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("summaries (-want +got):\n%s", diff)
	}
}

// A summary counts in the singular and the plural, and leaves out what the change holds none of.
func TestSummaryCountsEveryPart(t *testing.T) {
	tests := []struct {
		verb, name string
		pv         Preview
		want       string
	}{
		{"Adds", fixture.Feature, Preview{Features: 1, Hosts: 1, Env: 1, Waiting: []string{fixture.Secret}}, "Adds " + fixture.Feature + " with 1 host and 1 env value, 1 secret waits for a value"},
		{"Adds", fixture.Feature, Preview{Features: 3, Hosts: 5, Env: 2}, "Adds 3 features with 5 hosts and 2 env values"},
		{"Removes", fixture.Feature, Preview{Features: 1, Env: 3}, "Removes " + fixture.Feature + " with 3 env values"},
		{"Removes", fixture.Feature, Preview{Features: 1}, "Removes " + fixture.Feature},
	}
	// Write every change
	for _, tt := range tests {
		got := summary(tt.verb, tt.name, tt.pv)

		// Report a summary that differs
		if got != tt.want {
			t.Errorf("summary(%+v) = %q, want %q", tt.pv, got, tt.want)
		}
	}
}

// A feature dialog with nothing to choose from ends with a note.
func TestFeatureDialogsWithNothingToChooseEndWithANote(t *testing.T) {
	h := Home{Dir: "/home/ubuntu/o3s", Lookup: fakeLookup{}}
	_, add := walk(t, h, []string{"feature"}, []string{"add"})
	_, rm := walk(t, h, []string{"feature"}, []string{"rm"})

	got := [][]Command{add, rm}

	want := [][]Command{{{Note: "No store to choose from"}}, {{Note: "No feature to remove"}}}
	// Report notes that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("notes (-want +got):\n%s", diff)
	}
}

// The features of a store come as a multi-select, and Apply? shows each chosen one's change in turn.
func TestFeatureAddChoosesSeveral(t *testing.T) {
	asked, _ := walk(t, aHomeWithTheTool(), []string{"feature"}, []string{"add"}, []string{fixture.Store}, []string{fixture.OtherFeature, fixture.Feature}, []string{"no"})

	got := []any{asked[3].Kind, asked[4].Preview}

	want := []any{Multi, []string{
		otherPkg + ":" + fixture.Major, hostLine, otherHostLine,
		toolPkg + ":" + fixture.Major, hostLine, otherHostLine,
	}}
	// Report a question or a preview that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("features question (-want +got):\n%s", diff)
	}
}
