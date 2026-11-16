package menu

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fakeLookup is a store that offers the tool, whose feature needs two secrets still waiting for values.
type fakeLookup struct{}

// Features lists the ids the store offers, and fails for any other store.
func (fakeLookup) Features(store string) ([]string, error) {
	// Turn away a store out of reach
	if store != "registry.example.com/features" {
		return nil, errors.New("read the features of " + store + ": unreachable")
	}
	return []string{"other", "tool"}, nil
}

// Add shows what a feature brings, the tool with the options it offers.
func (fakeLookup) Add(id string) (Preview, error) {
	pv := Preview{Lines: []string{id + ":1", "  + example.com:443  EXAMPLE_TOKEN !", "  + example.org:443  OTHER_TOKEN !"}, Waiting: []string{"EXAMPLE_TOKEN", "OTHER_TOKEN"}}
	// Offer the tool's options alone
	if id == "registry.example.com/features/tool" {
		pv.Options = []FeatureOption{
			{Name: "debug", Type: "boolean", Default: "false", Description: "Log every step"},
			{Name: "mode", Type: "string", Default: "strict", Enum: []string{"strict", "loose"}},
			{Name: "version", Type: "string", Default: "latest", Proposals: []string{"latest", "lts"}},
		}
	}
	return pv, nil
}

// Remove shows what leaves with the tool.
func (fakeLookup) Remove(pkg string) (Preview, error) {
	return Preview{Lines: []string{pkg, "  - example.com:443"}}, nil
}

// aHomeWithTheTool is aHome holding the tool and reading its store through the fake.
func aHomeWithTheTool() Home {
	h := aHome
	h.Features = []string{"registry.example.com/features/tool"}
	h.Lookup = fakeLookup{}
	return h
}

// The feature dialogs preview their change, ask for waiting values, and run the commands they lead to.
func TestFeatureDialogsEndInTheirCommands(t *testing.T) {
	value := "s3cr3t"
	tests := []struct {
		name    string
		answers [][]string
		titles  []string
		want    []Command
	}{
		{
			"adding a feature with one value given and one skipped, its options kept",
			[][]string{{"feature"}, {"add"}, {"registry.example.com/features"}, {"tool"}, {"yes"}, {"keep defaults"}, {value}, {""}},
			[]string{"What do you want to do?", "Action", "Store", "Features", "Apply?", "Options", "Value for EXAMPLE_TOKEN (empty to skip)", "Value for OTHER_TOKEN (empty to skip)"},
			[]Command{
				{Args: []string{"secret", "add", "EXAMPLE_TOKEN", "--no-bind-hint"}, Stdin: &value},
				{Args: []string{"feature", "add", "registry.example.com/features/tool", "--no-preview"}},
			},
		},
		{
			"configuring every option a feature offers, each written as answered",
			[][]string{{"feature"}, {"add"}, {"registry.example.com/features"}, {"tool"}, {"yes"}, {"configure"}, {"true"}, {"loose"}, {"lts"}, {""}, {""}},
			[]string{"What do you want to do?", "Action", "Store", "Features", "Apply?", "Options", "tool debug", "tool mode", "tool version", "Value for EXAMPLE_TOKEN (empty to skip)", "Value for OTHER_TOKEN (empty to skip)"},
			[]Command{{Args: []string{"feature", "add", "registry.example.com/features/tool", "--option", "registry.example.com/features/tool:debug=true", "--option", "registry.example.com/features/tool:mode=loose", "--option", "registry.example.com/features/tool:version=lts", "--no-preview"}}},
		},
		{
			"declining a feature",
			[][]string{{"feature"}, {"add"}, {"registry.example.com/features"}, {"tool"}, {"no"}},
			[]string{"What do you want to do?", "Action", "Store", "Features", "Apply?"},
			nil,
		},
		{
			"adding two features that wait for the same secrets",
			[][]string{{"feature"}, {"add"}, {"registry.example.com/features"}, {"other", "tool"}, {"yes"}, {"keep defaults"}, {value}, {""}},
			[]string{"What do you want to do?", "Action", "Store", "Features", "Apply?", "Options", "Value for EXAMPLE_TOKEN (empty to skip)", "Value for OTHER_TOKEN (empty to skip)"},
			[]Command{
				{Args: []string{"secret", "add", "EXAMPLE_TOKEN", "--no-bind-hint"}, Stdin: &value},
				{Args: []string{"feature", "add", "registry.example.com/features/other", "registry.example.com/features/tool", "--no-preview"}},
			},
		},
		{
			"choosing no feature",
			[][]string{{"feature"}, {"add"}, {"registry.example.com/features"}, {}},
			[]string{"What do you want to do?", "Action", "Store", "Features"},
			[]Command{{Note: "No feature chosen"}},
		},
		{
			"a store out of reach",
			[][]string{{"feature"}, {"add"}, {"registry.example.org/features"}},
			[]string{"What do you want to do?", "Action", "Store"},
			[]Command{{Note: "error: read the features of registry.example.org/features: unreachable"}},
		},
		{
			"removing a feature",
			[][]string{{"feature"}, {"rm"}, {"registry.example.com/features/tool"}, {"yes"}},
			[]string{"What do you want to do?", "Action", "Feature", "Apply?"},
			[]Command{{Args: []string{"feature", "rm", "registry.example.com/features/tool", "--no-preview"}}},
		},
		{
			"listing the features",
			[][]string{{"feature"}, {"ls"}},
			[]string{"What do you want to do?", "Action"},
			[]Command{{Args: []string{"feature", "ls"}}},
		},
	}
	h := aHomeWithTheTool()
	h.Stores = append(h.Stores, "registry.example.org/features")
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
	asked, _ := walk(t, aHomeWithTheTool(), []string{"feature"}, []string{"add"}, []string{"registry.example.com/features"}, []string{"tool"}, []string{"yes"}, []string{"configure"}, []string{"false"}, []string{"strict"}, []string{"latest"}, []string{""}, []string{""})

	got := asked[6:9]

	want := []Question{
		{Key: "option:registry.example.com/features/tool:debug", Kind: Select, Title: "tool debug", Options: []Option{{Value: "true"}, {Value: "false"}}, Default: []string{"false"}, Preview: []string{"Log every step"}},
		{Key: "option:registry.example.com/features/tool:mode", Kind: Select, Title: "tool mode", Options: []Option{{Value: "strict"}, {Value: "loose"}}, Default: []string{"strict"}},
		{Key: "option:registry.example.com/features/tool:version", Kind: Input, Title: "tool version", Default: []string{"latest"}, Preview: []string{"suggested: latest, lts"}},
	}
	// Report questions that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("option questions (-want +got):\n%s", diff)
	}
}

// Apply? shows what the feature brings or takes, as the CLI prints it.
func TestFeatureApplyShowsThePreview(t *testing.T) {
	h := aHomeWithTheTool()
	added, _ := walk(t, h, []string{"feature"}, []string{"add"}, []string{"registry.example.com/features"}, []string{"tool"}, []string{"no"})
	removed, _ := walk(t, h, []string{"feature"}, []string{"rm"}, []string{"registry.example.com/features/tool"}, []string{"no"})

	got := [][]string{added[4].Preview, removed[3].Preview}

	want := [][]string{
		{"registry.example.com/features/tool:1", "  + example.com:443  EXAMPLE_TOKEN !", "  + example.org:443  OTHER_TOKEN !"},
		{"registry.example.com/features/tool", "  - example.com:443"},
	}
	// Report previews that differ
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("previews (-want +got):\n%s", diff)
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
	asked, _ := walk(t, aHomeWithTheTool(), []string{"feature"}, []string{"add"}, []string{"registry.example.com/features"}, []string{"other", "tool"}, []string{"no"})

	got := []any{asked[3].Kind, asked[4].Preview}

	want := []any{Multi, []string{
		"registry.example.com/features/other:1", "  + example.com:443  EXAMPLE_TOKEN !", "  + example.org:443  OTHER_TOKEN !",
		"registry.example.com/features/tool:1", "  + example.com:443  EXAMPLE_TOKEN !", "  + example.org:443  OTHER_TOKEN !",
	}}
	// Report a question or a preview that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("features question (-want +got):\n%s", diff)
	}
}
