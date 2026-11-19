// Package menu leads through the views and dialogs of bare o3s, each ending in the command lines the CLI runs.
package menu

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hansehart/o3s/internal/config"
)

// Home is what the menu knows about the home it runs in when it starts, and where it asks the stores.
type Home struct {
	Dir string
	// Exists marks a folder that holds a home already.
	Exists bool
	// Now is the moment the menu reads the home at, which end times count from.
	Now   time.Time
	Hosts []config.Host
	// Traffic is what the gateway recorded for each allowed host, by name.
	Traffic map[string]Traffic
	Secrets []Secret
	Stores  []string
	// Installed are the features the cage installs, in the order it installs them.
	Installed []Installed
	// Blocked are the names the gateway refused, each noting how often and when.
	Blocked []Option
	// Running marks a stack whose gateway answers, which alone knows what it blocked.
	Running bool
	// Stack names whether each part of the stack runs.
	Stack []Fact
	// Projects are the projects the workspace lists, as host/owner/name.
	Projects []string
	Lookup   Lookup
}

// Traffic is what the gateway recorded for one host, as the screen shows it.
type Traffic struct {
	Data, Seen string
}

// Installed is a feature the cage installs, with the tag it follows and the options it sets.
type Installed struct {
	Package, Tag string
	Options      map[string]string
}

// Lookup asks the stores what they offer and works out what a change brings to the home.
type Lookup interface {
	// Catalog lists the features a store offers.
	Catalog(store string) ([]Offer, error)
	// Add shows what adding a feature brings.
	Add(id string) (Preview, error)
	// Remove shows what leaves with a feature.
	Remove(pkg string) (Preview, error)
	// Updates lists every part with a newer version: o3s, the stack and each feature.
	Updates() ([]Update, error)
}

// Offer is a feature a store offers, with how it names it and what it does.
type Offer struct {
	ID, Name, Description string
}

// Update is a part with a newer version: the name update takes it by, how the screen names it, and what changes.
type Update struct {
	Target, Name, Change string
}

// Preview is the lines that show a change before it applies, and the secrets it waits for.
type Preview struct {
	Lines   []string
	Waiting []string
	// Options are the settings the feature offers for the user to set.
	Options []FeatureOption
	// Features, Hosts and Env count what the change adds or takes away.
	Features, Hosts, Env int
}

// FeatureOption is a setting a feature offers, with the values it takes and the one it falls back to.
type FeatureOption struct {
	Name, Type, Default string
	Enum, Proposals     []string
	Description         string
}

// Secret is a secret the home knows, and whether it holds a value.
type Secret struct {
	Name string
	Set  bool
}

// Kind is the way a question takes its answer.
type Kind int

const (
	// Select takes one of the options.
	Select Kind = iota
	// Multi takes any number of the options.
	Multi
	// Input takes a line of text.
	Input
	// Hidden takes a line of text the screen keeps hidden.
	Hidden
	// Confirm takes yes or no.
	Confirm
	// View shows a group's tabs and takes the tab with the row or the key the user picks in it.
	View
)

// Question is one step of a dialog.
type Question struct {
	Key   string
	Kind  Kind
	Title string
	// About says in one line what the question is about.
	About   string
	Options []Option
	Default []string
	Preview []string
	// Summary sums up the preview in one line, which stays in view beside the question.
	Summary string
	// Facts are the labeled facts a detail shows above its actions.
	Facts []Fact
	// Example shows the form a line of text takes.
	Example string
	// Check reads a line of text as the user types it, in plain words where it holds, and names the rule it breaks
	// otherwise.
	Check func(string) (string, error)
	// Numbered numbers the options, so a digit picks one.
	Numbered bool
	// Tabs are the tabs a view shows. A view's default names the tab it opens at and the row it starts on.
	Tabs []Tab
}

// Fact is one labeled fact a detail shows.
type Fact struct {
	Label, Value string
}

// Tab is one tab of a view: what it holds, what it says while it holds nothing, and how its rows answer.
type Tab struct {
	Name, About, Empty string
	Options            []Option
	// Multi marks a tab whose rows the user marks and hands back together, the ones in Marked marked from the start.
	Multi  bool
	Marked []string
	// Reorder lets the user move a row up or down.
	Reorder bool
	// Keys are the single keys the tab acts on beside Enter.
	Keys []Key
}

// Key is a single key a tab acts on, the help it shows, and the value it answers with.
type Key struct {
	Key, Help, Value string
}

// Option is one choice of a question, shown by its label with its columns lined up and a line that says what it is.
type Option struct {
	Value string
	// Label is how the option shows, its value where it has none.
	Label string
	Note  string
	// Columns line up beside the label across the rows of a list.
	Columns []string
	// Detail says in a line below the option what it is.
	Detail string
}

// Shown is how an option shows, its label or else its value.
func (o Option) Shown() string {
	// Take the label where the option has one
	if o.Label != "" {
		return o.Label
	}
	return o.Value
}

// Answer is what the user gave one question.
type Answer struct {
	Key    string
	Values []string
}

// Answers are the answers given so far, in the order the dialog asked.
type Answers []Answer

// With hands back the answers with one more.
func (a Answers) With(key string, values []string) Answers {
	return append(slices.Clone(a), Answer{Key: key, Values: values})
}

// has reports whether a question was answered.
func (a Answers) has(key string) bool {
	return slices.ContainsFunc(a, func(x Answer) bool { return x.Key == key })
}

// get hands back the first value given a question.
func (a Answers) get(key string) string {
	values := a.all(key)
	// Read a question without a value as empty
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// all hands back every value given a question.
func (a Answers) all(key string) []string {
	i := slices.IndexFunc(a, func(x Answer) bool { return x.Key == key })
	// Read an unanswered question as empty
	if i < 0 {
		return nil
	}
	return a[i].Values
}

// Command is the command line a dialog ends in, the value it hands on stdin, and a note where it runs none.
type Command struct {
	Args  []string
	Stdin *string
	Note  string
	// Terminal marks a command that needs the real terminal: to ask for sudo or to hand it to a shell.
	Terminal bool
	// Quiet marks a command whose success shows as the view it changed, without its output.
	Quiet bool
}

// Values a dialog answers with beside the rows it shows.
const (
	none  = "none"
	fresh = "+new"
	typed = "+type"
	yes   = "yes"
	add   = "+add"
	back  = "+back"
	// always keeps a host open without an end time.
	always = "always"
)

// nothingChanged is the note a declined change ends in.
const nothingChanged = "Nothing changed."

// Next hands back the question a dialog asks next, or reports that it asks nothing more.
func Next(h Home, a Answers) (Question, bool) {
	return next(h, a)
}

// Plan builds the command lines the answers lead to, in the order they run.
func Plan(h Home, a Answers) []Command {
	return plan(h, a)
}

// Open is the answers that open the menu at a group's view, the stores opening at the feature view's Stores tab.
func Open(path ...string) Answers {
	var a Answers
	// Open at the group the path names
	if len(path) > 0 {
		a = a.With("do", []string{path[0]})
	}
	// Open at the feature view's Stores tab for the stores
	if len(path) > 1 && path[0] == "feature" && path[1] == "store" {
		a = a.With("tab", []string{storesTab})
	}
	return a
}

// mainMenu offers the groups that work where the menu runs, each noting what it holds: inside a home every group but
// init, outside one init and the secrets.
func mainMenu(h Home) Question {
	entries := []Option{
		{Value: "init", Note: "make this folder a home"},
		{Value: "cage", Note: "VM, stack and cage · " + stackState(h)},
		{Value: "feature", Note: fmt.Sprintf("tools in the cage · %d installed", len(h.Installed))},
		{Value: "firewall", Note: fmt.Sprintf("hosts the cage reaches · %d allowed · %d blocked", len(h.Hosts), len(h.Blocked))},
		{Value: "project", Note: fmt.Sprintf("repositories in the workspace · %d", len(h.Projects))},
		{Value: "secret", Note: fmt.Sprintf("values the proxy injects · %d", len(h.Secrets))},
		{Value: "update", Note: "o3s, the stack and the features"},
	}
	q := Question{Key: "do", Kind: Select, Title: "What do you want to do?"}
	// Offer every group that works here
	for _, e := range entries {
		// Take init outside a home, every other group inside one, and the secrets in both
		if e.Value == "init" && !h.Exists || e.Value != "init" && (h.Exists || e.Value == "secret") {
			q.Options = append(q.Options, e)
		}
	}
	return q
}

// stackState names in a word whether the cage runs.
func stackState(h Home) string {
	// Read the cage's own part of the stack
	for _, f := range h.Stack {
		// Name the state of the cage
		if f.Label == "Cage" {
			return f.Value
		}
	}
	return "stopped"
}

// next hands back the question a dialog asks next, or reports that it asks nothing more.
func next(h Home, a Answers) (Question, bool) {
	// Ask what to do first
	if !a.has("do") {
		return mainMenu(h), false
	}
	// Follow the dialog of the group
	switch a.get("do") {
	case "init":
		return ask(a, Question{Key: "init", Kind: Confirm, Title: "Apply?", Summary: "Creates an o3s home in " + h.Dir, Default: []string{yes}})
	case "cage":
		return cageDialog(h, a)
	case "feature":
		return featureDialog(h, a)
	case "firewall":
		return firewallDialog(h, a)
	case "project":
		return projectDialog(h, a)
	case "secret":
		return secretDialog(h, a)
	case "update":
		return updateDialog(h, a)
	}
	return Question{}, true
}

// plan builds the command lines the answers lead to, in the order they run.
func plan(h Home, a Answers) []Command {
	// Build the commands of the group's dialog
	switch a.get("do") {
	case "init":
		// End where the user declines
		if a.get("init") != yes {
			return declined()
		}
		return []Command{{Args: []string{"init"}, Terminal: true}}
	case "cage":
		return cagePlan(a)
	case "feature":
		return featurePlan(h, a)
	case "firewall":
		return firewallPlan(h, a)
	case "project":
		return projectPlan(a)
	case "secret":
		return secretPlan(a)
	case "update":
		return updatePlan(h, a)
	}
	return nil
}

// view hands back what the user picked in a view: the tab and the values the row or the key answered with.
func view(a Answers) (string, []string) {
	v := a.all("view")
	// Read a view not yet answered as empty
	if len(v) == 0 {
		return "", nil
	}
	return v[0], v[1:]
}

// declined is the note a change ends in where the user says no.
func declined() []Command {
	return []Command{{Note: nothingChanged}}
}

// confirmed reports whether the user said yes to the change.
func confirmed(a Answers) bool {
	return a.get("apply") == yes
}

// apply asks to apply a change, showing it above the question and summing it up beside it. A change that takes
// something away starts on no.
func apply(lines []string, summary string, removes bool) Question {
	q := Question{Key: "apply", Kind: Confirm, Title: "Apply?", Preview: lines, Summary: summary, Default: []string{yes}}
	// Start a removal on no, so Enter alone keeps everything
	if removes {
		q.Default = []string{"no"}
	}
	return q
}

// detail asks what to do with one row of a view, showing its facts above its actions.
func detail(title string, facts []Fact, actions ...Option) Question {
	return Question{Key: "action", Kind: Select, Title: title, Facts: facts, Options: append(actions, Option{Value: back, Label: "Back"})}
}

// ask hands back a question until it is answered, and then ends the dialog.
func ask(a Answers, q Question) (Question, bool) {
	// End the dialog once the question is answered
	if a.has(q.Key) {
		return Question{}, true
	}
	return q, false
}

// count writes a number with the word that fits it.
func count(n int, one, many string) string {
	// Take the singular for a single one
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// joinAnd joins names the way a sentence lists them.
func joinAnd(names []string) string {
	// Join a single name as it stands, and the last of several with and
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
