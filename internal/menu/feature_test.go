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

// fakeLookup is a store that offers the tool and another feature, the tool needing two secrets still waiting for
// values, and newer versions of the cli, the stack and the tool.
type fakeLookup struct{}

// Catalog lists the features the store offers, and fails for any other store.
func (fakeLookup) Catalog(store string) ([]Offer, error) {
	// Turn away a store out of reach
	if store != fixture.Store {
		return nil, errors.New("read the features of " + store + ": unreachable")
	}
	return []Offer{{ID: fixture.OtherFeature, Description: "Another tool"}, {ID: fixture.Feature, Name: "Tool"}}, nil
}

// Peek lists the features the store offers at once, as a catalog read already.
func (l fakeLookup) Peek(store string) ([]Offer, bool, error) {
	catalog, err := l.Catalog(store)
	return catalog, true, err
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

// Remove shows what leaves with a feature.
func (fakeLookup) Remove(pkg string) (Preview, error) {
	return Preview{Lines: []string{pkg, goneLine}, Features: 1, Hosts: 1}, nil
}

// Updates lists newer versions of the cli, the stack and the tool.
func (fakeLookup) Updates() ([]Update, error) {
	return []Update{{Target: CLI, Name: CLI, Change: fixture.Version + " -> " + fixture.NewerVersion}, {Target: "stack", Name: "stack", Change: "1 file, 2 images"}, {Target: toolPkg, Name: fixture.Feature, Change: fixture.Major + " -> " + fixture.NewerMajor}}, nil
}

// aHomeWithTheTool is aHome holding the tool with its mode set.
func aHomeWithTheTool() Home {
	h := aHome
	h.Installed = []Installed{{Package: toolPkg, Tag: fixture.Major, Options: map[string]string{"mode": "loose"}}}
	return h
}

// The feature view shows the installed features in order to move, every offered feature to mark with the installed
// ones marked, and the stores.
func TestTheFeatureViewShowsInstalledDiscoverAndStores(t *testing.T) {
	q, _ := Next(aHomeWithTheTool(), Answers{}.With("do", []string{"feature"}))

	installed, discover, stores := q.Tabs[0], q.Tabs[1], q.Tabs[2]
	// Require the installed tool, movable
	if !installed.Reorder || len(installed.Options) != 1 || installed.Options[0].Value != toolPkg {
		t.Errorf("Installed = %+v", installed)
	}
	// Require both features offered, the tool marked, each saying what it is
	if !discover.Multi || len(discover.Options) != 2 || !cmp.Equal([]string{toolPkg}, discover.Marked) || discover.Options[0].Detail != "Another tool" || discover.Options[1].Detail != "Tool" {
		t.Errorf("Discover = %+v", discover)
	}
	// Require the add row first, then the store
	if stores.Options[0].Value != add || stores.Options[1].Value != fixture.Store {
		t.Errorf("Stores = %+v", stores)
	}
}

// Every feature dialog ends in the command lines the CLI runs for it, its confirmation last.
func TestEveryFeatureDialogEndsInItsCommand(t *testing.T) {
	value := fixture.SecretValue
	dialogs{
		{
			"setting one option of the tool, quietly",
			[][]string{{"feature"}, {installedTab, toolPkg}, {configure}, {"Options", "debug"}, {"true"}},
			[]string{"do", "view", "action", "options", "option:debug"},
			[]Command{{Args: []string{"feature", "configure", toolPkg, "--option", "debug=true"}, Quiet: true}},
		},
		{
			"keeping the value an option holds",
			[][]string{{"feature"}, {installedTab, toolPkg}, {configure}, {"Options", "mode"}, {"loose"}},
			[]string{"do", "view", "action", "options", "option:mode"},
			nil,
		},
		{
			"removing the tool",
			[][]string{{"feature"}, {installedTab, toolPkg}, {remove}, {yes}},
			[]string{"do", "view", "action", "apply"},
			[]Command{{Args: []string{"feature", "rm", toolPkg, "--no-preview", "--yes"}}},
		},
		{
			"ordering the features, quietly",
			[][]string{{"feature"}, {installedTab, Order, toolPkg, toolPkg}},
			[]string{"do", "view"},
			nil,
		},
		{
			"marking the other feature and unmarking the tool, a waiting secret given and one skipped",
			[][]string{{"feature"}, {discoverTab, otherPkg}, {value}, {""}, {yes}},
			[]string{"do", "view", "value:" + fixture.Secret, "value:" + fixture.OtherSecret, "apply"},
			[]Command{
				{Args: []string{"secret", "add", fixture.Secret, "--no-bind-hint"}, Stdin: &value},
				{Args: []string{"feature", "add", otherPkg, "--no-preview"}},
				{Args: []string{"feature", "rm", toolPkg, "--no-preview", "--yes"}},
			},
		},
		{
			"every mark taken off, removing the installed feature",
			[][]string{{"feature"}, {discoverTab}, {yes}},
			[]string{"do", "view", "apply"},
			[]Command{{Args: []string{"feature", "rm", toolPkg, "--no-preview", "--yes"}}},
		},
		{
			"marks that change nothing",
			[][]string{{"feature"}, {discoverTab, toolPkg}},
			[]string{"do", "view"},
			[]Command{{Note: nothingChanged}},
		},
		{
			"adding a store",
			[][]string{{"feature"}, {storesTab, add}, {fixture.AbsentStore}},
			[]string{"do", "view", "store"},
			[]Command{{Args: []string{"feature", "store", "add", fixture.AbsentStore}}},
		},
		{
			"removing a store",
			[][]string{{"feature"}, {storesTab, fixture.Store}, {remove}, {yes}},
			[]string{"do", "view", "action", "apply"},
			[]Command{{Args: []string{"feature", "store", "rm", fixture.Store, "--yes"}}},
		},
	}.check(t, aHomeWithTheTool())
}

// A new order of the installed features runs one quiet feature order over all of them, and nothing where the order
// stays.
func TestOrderingFeaturesRunsOneQuietOrder(t *testing.T) {
	h := aHomeWithTheTool()
	h.Installed = append(h.Installed, Installed{Package: otherPkg, Tag: fixture.Major})
	ordered := func(order ...string) []Command {
		return Plan(h, Answers{}.With("do", []string{"feature"}).With("view", append([]string{installedTab, Order, toolPkg}, order...)))
	}

	// Require one quiet order with the tool second
	if diff := cmp.Diff([]Command{{Args: []string{"feature", "order", otherPkg, toolPkg}, Quiet: true}}, ordered(otherPkg, toolPkg)); diff != "" {
		t.Errorf("new order (-want +got):\n%s", diff)
	}
	// Require nothing for the order the features have
	if got := ordered(toolPkg, otherPkg); got != nil {
		t.Errorf("same order = %v, want nothing", got)
	}
}

// The confirmation of marks shows what the new features bring and what leaves with the unmarked ones, summed up, and
// starts on no where something leaves.
func TestMarkingFeaturesConfirmsTheWholeChange(t *testing.T) {
	asked, _ := walk(t, aHomeWithTheTool(), []string{"feature"}, []string{discoverTab, otherPkg}, []string{""}, []string{""}, []string{"no"})

	got := asked[4]
	// Require the lines of both changes
	if diff := cmp.Diff([]string{otherPkg + ":" + fixture.Major, hostLine, otherHostLine, toolPkg, goneLine}, got.Preview); diff != "" {
		t.Errorf("preview (-want +got):\n%s", diff)
	}
	// Require the summary of both, and no as the start
	if want := "Adds " + fixture.OtherFeature + " with 2 hosts, 2 secrets wait for a value, removes 1 feature"; got.Summary != want || got.Default[0] != "no" {
		t.Errorf("summary = %q, default %v, want %q and no", got.Summary, got.Default, want)
	}
}

// An option asks the way it takes its value, starting from the value the feature holds and marking it.
func TestOptionsAskFromTheValueTheyHold(t *testing.T) {
	configuring := Answers{}.With("do", []string{"feature"}).With("view", []string{installedTab, toolPkg}).With("action", []string{configure})
	ask := func(name string) Question {
		q, _ := Next(aHomeWithTheTool(), configuring.With("options", []string{"Options", name}))
		return q
	}

	debug, mode, version := ask("debug"), ask("mode"), ask("version")

	// Require a boolean as a choice of true and false, from its fallback
	if debug.Kind != Select || debug.Default[0] != "false" || debug.About != "Log every step" {
		t.Errorf("debug = %+v", debug)
	}
	// Require a choice from the value the feature holds, marked current
	if mode.Kind != Select || mode.Default[0] != "loose" || mode.Options[1].Note != "current" {
		t.Errorf("mode = %+v", mode)
	}
	// Require a line of text with its proposals as the example and the values it completes to
	if version.Kind != Input || version.Example != fixture.FloatingTag+"  lts" || !cmp.Equal([]string{fixture.FloatingTag, "lts"}, version.Suggestions) {
		t.Errorf("version = %+v", version)
	}
}

// A feature's options show in one view, each with the value the feature sets, its default and what it does.
func TestTheOptionsViewShowsEveryOption(t *testing.T) {
	got, _ := Next(aHomeWithTheTool(), Answers{}.With("do", []string{"feature"}).With("view", []string{installedTab, toolPkg}).With("action", []string{configure}))

	// Require one row per option with its value, default and description
	want := []Option{
		{Value: "debug", Columns: []string{"–", "false"}, Detail: "Log every step"},
		{Value: "mode", Columns: []string{"loose", "strict"}},
		{Value: "version", Columns: []string{"–", fixture.FloatingTag}},
	}
	// Report a view that differs
	if diff := cmp.Diff(want, got.Tabs[0].Options); got.Kind != View || diff != "" {
		t.Errorf("options view %v (-want +got):\n%s", got.Kind, diff)
	}
}

// lateLookup is the fake store whose catalog is still on its way.
type lateLookup struct{ fakeLookup }

// Peek reports the catalog still on its way.
func (lateLookup) Peek(string) ([]Offer, bool, error) {
	return nil, false, nil
}

// The feature view opens at once while the stores' catalogs come in: Discover says the features are on their way and
// the view waits for them, the installed features already in place.
func TestTheFeatureViewOpensBeforeTheStoresAnswer(t *testing.T) {
	h := aHomeWithTheTool()
	h.Lookup = lateLookup{}

	q, _ := Next(h, Answers{}.With("do", []string{"feature"}))

	installed, discover := q.Tabs[0], q.Tabs[1]
	// Require the installed feature in place and Discover on its way
	if len(installed.Options) != 1 || !discover.Loading || discover.Empty != "Loading the stores' features…" || !q.Pending() {
		t.Errorf("view = %+v, pending %v", q.Tabs, q.Pending())
	}
}

// A store's row names how many features it offers and how many of them the cage installs.
func TestAStoresRowShowsItsFeatures(t *testing.T) {
	q, _ := Next(aHomeWithTheTool(), Answers{}.With("do", []string{"feature"}).With("view", []string{storesTab, fixture.Store}))

	// Require the store's facts
	if diff := cmp.Diff([]Fact{{Label: "Features", Value: "2"}, {Label: "Installed from it", Value: "1"}}, q.Facts); diff != "" {
		t.Errorf("store facts (-want +got):\n%s", diff)
	}
}
