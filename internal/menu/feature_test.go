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
// values, and newer versions of o3s, the stack and the tool.
type fakeLookup struct{}

// Catalog lists the features the store offers, and fails for any other store.
func (fakeLookup) Catalog(store string) ([]Offer, error) {
	// Turn away a store out of reach
	if store != fixture.Store {
		return nil, errors.New("read the features of " + store + ": unreachable")
	}
	return []Offer{{ID: fixture.OtherFeature, Description: "Another tool"}, {ID: fixture.Feature, Name: "Tool"}}, nil
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

// Updates lists newer versions of o3s, the stack and the tool.
func (fakeLookup) Updates() ([]Update, error) {
	return []Update{{Target: "o3s", Name: "o3s", Change: fixture.Version + " -> " + fixture.NewerVersion}, {Target: "stack", Name: "stack", Change: "1 file, 2 images"}, {Target: toolPkg, Name: fixture.Feature, Change: fixture.Major + " -> " + fixture.NewerMajor}}, nil
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
			"configuring the tool, its mode back to strict",
			[][]string{{"feature"}, {installedTab, toolPkg}, {configure}, {"true"}, {"strict"}, {fixture.FloatingTag}, {yes}},
			[]string{"do", "view", "action", "option:debug", "option:mode", "option:version", "apply"},
			[]Command{{Args: []string{"feature", "configure", toolPkg, "--option", "debug=true", "--option", "mode=strict"}}},
		},
		{
			"configuring the tool without a change",
			[][]string{{"feature"}, {installedTab, toolPkg}, {configure}, {"false"}, {"loose"}, {fixture.FloatingTag}, {yes}},
			[]string{"do", "view", "action", "option:debug", "option:mode", "option:version", "apply"},
			[]Command{{Note: nothingChanged}},
		},
		{
			"removing the tool",
			[][]string{{"feature"}, {installedTab, toolPkg}, {remove}, {yes}},
			[]string{"do", "view", "action", "apply"},
			[]Command{{Args: []string{"feature", "rm", toolPkg, "--no-preview", "--yes"}}},
		},
		{
			"moving the tool up, quietly",
			[][]string{{"feature"}, {installedTab, move, toolPkg, "up"}},
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

// Moving an installed feature orders the features again with it one place further, quietly, and leaves one at either
// end where it is.
func TestMovingAFeatureOrdersThemQuietly(t *testing.T) {
	h := aHomeWithTheTool()
	h.Installed = append(h.Installed, Installed{Package: otherPkg, Tag: fixture.Major})

	down := Plan(h, Answers{}.With("do", []string{"feature"}).With("view", []string{installedTab, move, toolPkg, "down"}))
	up := Plan(h, Answers{}.With("do", []string{"feature"}).With("view", []string{installedTab, move, toolPkg, "up"}))

	// Require the order with the tool second, quietly
	if diff := cmp.Diff([]Command{{Args: []string{"feature", "order", otherPkg, toolPkg}, Quiet: true}}, down); diff != "" {
		t.Errorf("down (-want +got):\n%s", diff)
	}
	// Require nothing for the first feature moving up
	if up != nil {
		t.Errorf("up = %v, want nothing", up)
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
	asked, _ := walk(t, aHomeWithTheTool(), []string{"feature"}, []string{installedTab, toolPkg}, []string{configure}, []string{"false"}, []string{"loose"}, []string{fixture.FloatingTag}, []string{"no"})

	debug, mode, version := asked[3], asked[4], asked[5]
	// Require a boolean as a choice of true and false, from its fallback
	if debug.Kind != Select || debug.Default[0] != "false" || debug.About != "Log every step" {
		t.Errorf("debug = %+v", debug)
	}
	// Require a choice from the value the feature holds, marked current
	if mode.Kind != Select || mode.Default[0] != "loose" || mode.Options[1].Note != "current" {
		t.Errorf("mode = %+v", mode)
	}
	// Require a line of text with its proposals as the example
	if version.Kind != Input || version.Example != fixture.FloatingTag+"  lts" {
		t.Errorf("version = %+v", version)
	}
}
