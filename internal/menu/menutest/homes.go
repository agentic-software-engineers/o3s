// Package menutest holds the homes the menu's tests run on and walks every path of the menu through them, the way
// registrytest raises a registry for the registry's tests.
package menutest

import (
	"errors"
	"strconv"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/vm"
)

// Dir is the folder every home of the tests lives in.
const Dir = fixture.HomeDir

// Tool is the package of the feature the store offers with options, a secret and hosts.
const Tool = fixture.Store + "/" + fixture.Feature

// Other is the package of the plain second feature the store offers.
const Other = fixture.Store + "/" + fixture.OtherFeature

// Home is a named home the menu runs on.
type Home struct {
	Name string
	Home menu.Home
}

// Homes are every home the menu runs on in the tests: outside a home, a full home with its stack running or stopped,
// an empty one, and a home in its VM in every state.
func Homes() []Home {
	stopped := full()
	stopped.Running = false
	stopped.Stack = stack("stopped")
	return []Home{
		{Name: "outside", Home: menu.Home{Dir: Dir, Now: fixture.Now, Secrets: secrets()}},
		{Name: "running", Home: full()},
		{Name: "stopped", Home: stopped},
		{Name: "empty", Home: menu.Home{Dir: Dir, Exists: true, Now: fixture.Now, Stack: stack("stopped"), Lookup: Lookup{}}},
		{Name: "vm-unfinished", Home: inVM(vm.Unfinished)},
		{Name: "vm-stopped", Home: inVM(vm.Stopped)},
		{Name: "vm-running", Home: inVM(vm.Running)},
	}
}

// full is a home on this machine that holds a bit of everything: hosts open for good, for a while and past their
// end, secrets set and waiting, a store, both its features installed, the tool with an option set, a refused name
// and a project.
func full() menu.Home {
	return menu.Home{
		Dir:    Dir,
		Exists: true,
		Now:    fixture.Now,
		Hosts: []config.Host{
			{Name: fixture.Host, Ports: []int{fixture.OtherPort, fixture.Port}, By: []string{"user"}},
			{Name: fixture.Subdomain, Ports: []int{fixture.Port}, Secret: fixture.Secret, Inject: []string{"header"}, Expires: fixture.InAnHourText, By: []string{"user"}},
			{Name: fixture.ThirdHost, Ports: []int{fixture.Port}, Expires: fixture.LongAgoText, By: []string{"user"}},
			{Name: fixture.OtherHost, Ports: []int{fixture.Port}, By: []string{Tool}},
		},
		Traffic:   map[string]menu.Traffic{fixture.Host: {Data: fixture.DataMoved, Seen: fixture.LastSeen}},
		Secrets:   secrets(),
		Stores:    []string{fixture.Store},
		Installed: []menu.Installed{{Package: Tool, Tag: fixture.Major, Options: map[string]string{fixture.Choice: fixture.OtherChoiceValue}}, {Package: Other, Tag: fixture.Major, Options: map[string]string{}}},
		Blocked:   []menu.Option{{Value: fixture.RefusedName, Columns: menu.BlockedRow(fixture.Attempts, fixture.LastSeen)}},
		Running:   true,
		Stack:     stack("running"),
		Projects:  []string{"github.com/" + fixture.Repo},
		Lookup:    Lookup{},
	}
}

// secrets are a secret waiting for its value and one that holds it.
func secrets() []menu.Secret {
	return []menu.Secret{{Name: fixture.Secret}, {Name: fixture.OtherSecret, Set: true}}
}

// stack is where every part of the stack stands, all in the same state.
func stack(state string) []menu.Fact {
	return []menu.Fact{{Label: "Cage", Value: state}, {Label: "Host", Value: state}}
}

// inVM is a home that lives in its VM, in the state given.
func inVM(state vm.State) menu.Home {
	return menu.Home{Dir: Dir, Now: fixture.Now, VM: state, Stack: []menu.Fact{{Label: "VM", Value: string(state)}}, Lookup: Lookup{}}
}

// Lookup is a store that offers the tool and another feature, the tool with an option of every kind and both waiting
// for a secret, and newer versions of the cli, the stack and the tool.
type Lookup struct{}

// Catalog lists the features the store offers, and fails for any other store.
func (Lookup) Catalog(store string) ([]menu.Offer, error) {
	// Turn away a store out of reach
	if store != fixture.Store {
		return nil, errors.New("read the features of " + store + ": unreachable")
	}
	return []menu.Offer{{ID: fixture.OtherFeature, Description: fixture.OtherFeatureAbout}, {ID: fixture.Feature, Name: fixture.FeatureTitle, Description: fixture.FeatureAbout}}, nil
}

// Peek lists the features the store offers at once, as a catalog read already.
func (l Lookup) Peek(store string) ([]menu.Offer, bool, error) {
	catalog, err := l.Catalog(store)
	return catalog, true, err
}

// Add shows what a feature brings, the tool with the options it offers.
func (Lookup) Add(id string) (menu.Preview, error) {
	pv := menu.Preview{Lines: []string{id + ":" + fixture.Major, "+ " + fixture.NewHost + ":" + strconv.Itoa(fixture.Port)}, Waiting: []string{fixture.Secret}, Features: 1, Hosts: 1}
	// Offer the tool's options alone
	if id == Tool {
		pv.Options = []menu.FeatureOption{
			{Name: fixture.Toggle, Type: "boolean", Default: "false", Description: fixture.ToggleAbout},
			{Name: fixture.Choice, Type: "string", Default: fixture.ChoiceValue, Enum: []string{fixture.ChoiceValue, fixture.OtherChoiceValue}, Description: fixture.ChoiceAbout},
			{Name: fixture.Free, Type: "string", Default: fixture.FloatingTag, Proposals: []string{fixture.FloatingTag, fixture.Proposal}, Description: fixture.FreeAbout},
		}
	}
	return pv, nil
}

// Remove shows what leaves with a feature.
func (Lookup) Remove(pkg string) (menu.Preview, error) {
	return menu.Preview{Lines: []string{"- " + pkg, "- " + fixture.OtherHost + ":" + strconv.Itoa(fixture.Port)}, Features: 1, Hosts: 1}, nil
}

// Updates lists newer versions of the cli, the stack and the tool.
func (Lookup) Updates() ([]menu.Update, error) {
	return []menu.Update{
		{Target: menu.CLI, Name: menu.CLI, Change: fixture.Version + " -> " + fixture.NewerVersion},
		{Target: "stack", Name: "stack", Change: fixture.StackChange},
		{Target: Tool, Name: fixture.Feature, Change: fixture.Major + " -> " + fixture.NewerMajor},
	}, nil
}
