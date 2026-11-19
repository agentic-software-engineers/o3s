package menu

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/words"
)

// The tabs of the feature view.
const (
	installedTab = "Installed"
	discoverTab  = "Discover"
	storesTab    = "Stores"
)

// Values the feature view and a feature's detail answer with.
const (
	configure = "configure"
)

// featureView shows the installed features in the order they install, every feature the stores offer, and the
// stores.
func featureView(h Home, a Answers) Question {
	installed := Tab{Name: installedTab, About: "Installed in this order on every rebuild. Pick one up with m to move it.", Empty: "No features yet. Mark the ones you want on the Discover tab.", Header: InstalledHeader, Reorder: true}
	// Show every installed feature as its row
	for _, f := range h.Installed {
		installed.Options = append(installed.Options, Option{Value: f.Package, Label: path.Base(f.Package), Columns: InstalledRow(f, h.Hosts)})
	}
	rows, loading := offers(h)
	discover := Tab{Name: discoverTab, About: "Every feature the stores offer. Mark the ones the cage installs.", Empty: "No store offers a feature. Add one on the Stores tab.", Header: []string{"FEATURE", "STORE"}, Multi: true, Options: rows, Marked: packages(h), Loading: loading}
	// Say that the features are on their way while the stores answer
	if loading && len(rows) == 0 {
		discover.Empty = "Loading the stores' features…"
	}
	stores := Tab{Name: storesTab, About: "Where features come from.", Options: []Option{{Value: add, Label: "+ Add a store…"}}}
	// Show every store
	for _, s := range h.Stores {
		stores.Options = append(stores.Options, Option{Value: s})
	}
	return Question{Key: "view", Kind: View, Title: "Feature", Tabs: []Tab{installed, discover, stores}, Default: []string{a.get("tab")}}
}

// offers lists every feature the stores offer, the installed ones among them, by store/id, and reports whether a store's
// catalog is still on its way. Peek takes what is read already, so the view shows it at once.
func offers(h Home) ([]Option, bool) {
	var out []Option
	loading := false
	// Take every store's catalog that is read
	for _, s := range h.Stores {
		catalog, read, err := h.Lookup.Peek(s)
		// Wait for a catalog on its way, and leave out a store out of reach
		if !read || err != nil {
			loading = loading || !read
			continue
		}
		out = append(out, offerRows(h, s, catalog)...)
	}
	return out, loading
}

// offerRows shows a store's features by their id, with what they do and the store, each installed one noted.
func offerRows(h Home, store string, catalog []Offer) []Option {
	out := make([]Option, 0, len(catalog))
	// Show every feature of the store
	for _, f := range catalog {
		o := Option{Value: store + "/" + f.ID, Label: f.ID, Detail: words.Or(f.Description, f.Name), Columns: []string{path.Base(path.Dir(store))}}
		// Note a feature the cage installs already
		if slices.Contains(packages(h), o.Value) {
			o.Note = "installed"
		}
		out = append(out, o)
	}
	return out
}

// offered lists every feature the stores offer, waiting for every catalog, as the changes a dialog plans need.
func offered(h Home) []Option {
	var out []Option
	// Take every store's catalog once it is read
	for _, s := range h.Stores {
		catalog, err := h.Lookup.Catalog(s)
		// Leave out a store out of reach
		if err != nil {
			continue
		}
		out = append(out, offerRows(h, s, catalog)...)
	}
	return out
}

// packages lists the packages of the installed features.
func packages(h Home) []string {
	out := make([]string, len(h.Installed))
	// Take every installed feature's package
	for i, f := range h.Installed {
		out[i] = f.Package
	}
	return out
}

// featureDialog asks what adding, configuring, ordering or removing features and stores needs, starting at the
// feature view.
func featureDialog(h Home, a Answers) (Question, bool) {
	// Show the view first
	if !a.has("view") {
		return featureView(h, a), false
	}
	tab, picked := view(a)
	// Follow what the user picked on its tab
	switch tab {
	case installedTab:
		// End a new order or an empty pick, which the plan runs or leaves
		if len(picked) == 0 || picked[0] == Order {
			return Question{}, true
		}
		return installedDialog(h, a, picked[0])
	case discoverTab:
		return discoverDialog(h, a, picked)
	case storesTab:
		return storeDialog(a, picked)
	}
	return Question{}, true
}

// installedDialog asks what to do with an installed feature, then what configuring or removing it needs.
func installedDialog(h Home, a Answers, pkg string) (Question, bool) {
	pv, err := h.Lookup.Add(pkg)
	// Show the feature's facts and what the user can do with it
	if !a.has("action") {
		var actions []Option
		// Offer to configure a feature that offers options
		if err == nil && len(pv.Options) > 0 {
			actions = append(actions, Option{Value: configure, Label: "Configure options"})
		}
		actions = append(actions, Option{Value: remove, Label: "Remove"})
		return detail(path.Base(pkg), featureFacts(h, pkg), actions...), false
	}
	// Follow what the user chose
	switch a.get("action") {
	case configure:
		// Ask every option in turn, starting from the value it holds
		for _, o := range pv.Options {
			// Ask an option not yet answered
			if !a.has(optionKey(o)) {
				return optionQuestion(o, current(h, pkg, o)), false
			}
		}
		return ask(a, apply(configureLines(h, a, pkg, pv.Options), "Configures "+path.Base(pkg)+", rebuild the cage to apply it", false))
	case remove:
		rm, err := h.Lookup.Remove(pkg)
		// End the dialog where the change stays unknown
		if err != nil {
			return Question{}, true
		}
		return ask(a, apply(rm.Lines, "Removes "+path.Base(pkg)+summaryTail(rm), true))
	}
	return Question{}, true
}

// discoverDialog works out what marking features adds and removes, asks the values of the secrets the new ones wait
// for, then the confirmation.
func discoverDialog(h Home, a Answers, marked []string) (Question, bool) {
	added, removed := changes(h, marked)
	// End where the marks change nothing
	if len(added) == 0 && len(removed) == 0 {
		return Question{}, true
	}
	pv, err := previewAll(h, added)
	// End the dialog where the change stays unknown
	if err != nil {
		return Question{}, true
	}
	// Ask the value of every secret the new features wait for, before the confirmation
	for _, name := range pv.Waiting {
		// Ask for a value not yet given
		if !a.has("value:" + name) {
			return Question{Key: "value:" + name, Kind: Hidden, Title: "Value for " + name, About: "Empty leaves it waiting, echo \"$TOKEN\" | o3s secret add " + name + " sets it later."}, false
		}
	}
	lines := slices.Clone(pv.Lines)
	var parts []string
	// Name what the new features bring
	if len(added) > 0 {
		parts = append(parts, summary("Adds", path.Base(added[0]), pv))
	}
	// Show and name what leaves with every unmarked feature
	for _, pkg := range removed {
		rm, err := h.Lookup.Remove(pkg)
		// Show a removal whose change stays unknown by its name alone
		if err != nil {
			rm.Lines = []string{pkg}
		}
		lines = append(lines, rm.Lines...)
	}
	// Name the features that leave
	if len(removed) > 0 {
		parts = append(parts, "removes "+words.Count(len(removed), "feature", "features"))
	}
	return ask(a, apply(lines, capitalized(strings.Join(parts, ", ")), len(removed) > 0))
}

// storeDialog asks the store to add, or what to do with a store, then the confirmation of its removal.
func storeDialog(a Answers, picked []string) (Question, bool) {
	// End an empty pick
	if len(picked) == 0 {
		return Question{}, true
	}
	// Ask the store to add
	if picked[0] == add {
		return ask(a, Question{Key: "store", Kind: Input, Title: "Store", About: "A registry path that holds dev container features.", Example: "ghcr.io/devcontainers/features", Check: checkStore})
	}
	// Ask what to do with the store
	if !a.has("action") {
		return detail(picked[0], nil, Option{Value: remove, Label: "Remove"}), false
	}
	// Confirm the removal, the features installed from the store staying
	if a.get("action") == remove {
		return ask(a, apply([]string{"- " + picked[0]}, "Removes the store, the features installed from it stay", true))
	}
	return Question{}, true
}

// featurePlan builds the commands the feature view's dialogs lead to.
func featurePlan(h Home, a Answers) []Command {
	tab, picked := view(a)
	// Go back to the view where the user picked nothing or stepped back
	if len(picked) == 0 || a.get("action") == back {
		return nil
	}
	// Order the features without asking, since moving them back undoes it
	if tab == installedTab && picked[0] == Order && len(picked) > 1 {
		return orderPlan(h, picked[2:])
	}
	// Add the store the user typed, which asks no confirmation since removing it undoes it
	if tab == storesTab && picked[0] == add {
		return []Command{{Args: []string{"feature", "store", "add", strings.TrimSpace(a.get("store"))}}}
	}
	// Run nothing where the marks change nothing
	if added, removed := changes(h, picked); tab == discoverTab && len(added) == 0 && len(removed) == 0 {
		return declined()
	}
	// Leave everything as it was where the user declines
	if !confirmed(a) {
		return declined()
	}
	// Build the commands of the tab
	switch tab {
	case installedTab:
		return installedPlan(h, a, picked[0])
	case discoverTab:
		return discoverPlan(h, a, picked)
	}
	return []Command{{Args: []string{"feature", "store", "rm", picked[0], "--yes"}}}
}

// installedPlan builds the configuration or the removal of an installed feature.
func installedPlan(h Home, a Answers, pkg string) []Command {
	// Remove the feature the user confirmed
	if a.get("action") == remove {
		return []Command{{Args: []string{"feature", "rm", pkg, "--no-preview", "--yes"}}}
	}
	pv, err := h.Lookup.Add(pkg)
	// Name a change that stays unknown
	if err != nil {
		return []Command{{Note: "error: " + err.Error()}}
	}
	args := []string{"feature", "configure", pkg}
	// Hand on every option whose value the user changed
	for _, o := range pv.Options {
		// Take an option that changes
		if v := a.get(optionKey(o)); v != current(h, pkg, o) {
			args = append(args, "--option", o.Name+"="+v)
		}
	}
	// Run nothing where no option changes
	if len(args) == 3 {
		return declined()
	}
	return []Command{{Args: args}}
}

// discoverPlan stores the values the user gave, then adds the marked features and removes the unmarked ones.
func discoverPlan(h Home, a Answers, marked []string) []Command {
	added, removed := changes(h, marked)
	pv, err := previewAll(h, added)
	// Name a change that stays unknown
	if err != nil {
		return []Command{{Note: "error: " + err.Error()}}
	}
	var cmds []Command
	// Store every value the user gave, so the feature finds it in place
	for _, name := range pv.Waiting {
		value := a.get("value:" + name)
		// Leave a skipped secret waiting
		if value != "" {
			cmds = append(cmds, Command{Args: []string{"secret", "add", name, "--no-bind-hint"}, Stdin: &value})
		}
	}
	// Add the newly marked features
	if len(added) > 0 {
		cmds = append(cmds, Command{Args: append(append([]string{"feature", "add"}, added...), "--no-preview")})
	}
	// Remove the unmarked ones
	if len(removed) > 0 {
		cmds = append(cmds, Command{Args: append(append([]string{"feature", "rm"}, removed...), "--no-preview", "--yes")})
	}
	return cmds
}

// orderPlan installs the features in the order the user dropped them in, at once and quietly, and runs nothing where
// the order stays as it was.
func orderPlan(h Home, order []string) []Command {
	// Run nothing for an order that stays
	if slices.Equal(order, packages(h)) {
		return nil
	}
	return []Command{{Args: append([]string{"feature", "order"}, order...), Quiet: true}}
}

// changes works out which marked features are new and which installed ones the stores offer and the user unmarked.
func changes(h Home, marked []string) ([]string, []string) {
	all := offered(h)
	var added, removed []string
	// Take every marked feature the cage lacks
	for _, id := range marked {
		// Take a feature not installed yet
		if !slices.Contains(packages(h), id) {
			added = append(added, id)
		}
	}
	// Take every installed feature the stores offer that the user unmarked
	for _, pkg := range packages(h) {
		// Take an unmarked feature the view showed
		if !slices.Contains(marked, pkg) && slices.ContainsFunc(all, func(o Option) bool { return o.Value == pkg }) {
			removed = append(removed, pkg)
		}
	}
	return added, removed
}

// featureFacts are the facts a feature's detail shows: its tag, its store and the options it sets.
func featureFacts(h Home, pkg string) []Fact {
	i := slices.IndexFunc(h.Installed, func(f Installed) bool { return f.Package == pkg })
	// Show nothing for a feature the home lacks
	if i < 0 {
		return nil
	}
	f := h.Installed[i]
	facts := []Fact{{Label: "Tag", Value: f.Tag}, {Label: "Store", Value: path.Dir(pkg)}}
	var hosts []string
	// Name every host the feature needs
	for _, host := range h.Hosts {
		// Take a host that names the feature among its reasons
		if slices.Contains(host.By, pkg) {
			hosts = append(hosts, host.Name)
		}
	}
	// Name the hosts the feature needs
	if len(hosts) > 0 {
		facts = append(facts, Fact{Label: "Hosts", Value: strings.Join(hosts, ", ")})
	}
	var set []string
	// Name every option the feature sets
	for _, name := range slices.Sorted(maps.Keys(f.Options)) {
		set = append(set, name+"="+f.Options[name])
	}
	// Name the options the feature sets
	if len(set) > 0 {
		facts = append(facts, Fact{Label: "Options", Value: strings.Join(set, " · ")})
	}
	return facts
}

// configureLines shows every option the user changed, with its value before and after.
func configureLines(h Home, a Answers, pkg string, options []FeatureOption) []string {
	var lines []string
	// Show every option that changes
	for _, o := range options {
		// Take an option whose value changes
		if v, was := a.get(optionKey(o)), current(h, pkg, o); v != was {
			lines = append(lines, fmt.Sprintf("~ %s %s -> %s", o.Name, words.Or(was, "none"), v))
		}
	}
	// Name that nothing changes
	if len(lines) == 0 {
		return []string{"Every option keeps its value"}
	}
	return lines
}

// current hands back the value an installed feature gives an option, or the one the option falls back to.
func current(h Home, pkg string, o FeatureOption) string {
	// Take the value the feature sets
	for _, f := range h.Installed {
		// Read the option of the matching feature
		if v, ok := f.Options[o.Name]; f.Package == pkg && ok {
			return v
		}
	}
	return o.Default
}

// optionKey names the answer to one option.
func optionKey(o FeatureOption) string {
	return "option:" + o.Name
}

// optionQuestion asks for an option the way it takes its value, starting from the value it holds.
func optionQuestion(o FeatureOption, value string) Question {
	q := Question{Key: optionKey(o), Title: o.Name, About: o.Description, Default: []string{value}, Numbered: true}
	// Ask by the kind of value the option takes
	switch {
	case o.Type == "boolean":
		q.Kind, q.Options = Select, []Option{{Value: "true"}, {Value: "false"}}
	case len(o.Enum) > 0:
		q.Kind = Select
		// Offer every value the option takes
		for _, v := range o.Enum {
			q.Options = append(q.Options, Option{Value: v})
		}
	default:
		q.Kind = Input
		// Show the values the option suggests
		if len(o.Proposals) > 0 {
			q.Example = strings.Join(o.Proposals, "  ")
		}
	}
	// Mark the value the option holds now
	for i := range q.Options {
		// Note the current value
		if q.Options[i].Value == value {
			q.Options[i].Note = "current"
		}
	}
	return q
}

// previewAll joins what every new feature brings, each secret they wait for named once.
func previewAll(h Home, ids []string) (Preview, error) {
	var all Preview
	// Preview every new feature in turn
	for _, id := range ids {
		pv, err := h.Lookup.Add(id)
		// Hand back a change that stays unknown
		if err != nil {
			return Preview{}, err
		}
		all.Lines = append(all.Lines, pv.Lines...)
		all.Features, all.Hosts, all.Env = all.Features+pv.Features, all.Hosts+pv.Hosts, all.Env+pv.Env
		// Take each waiting secret once
		for _, name := range pv.Waiting {
			// Keep a secret taken already out of the list
			if !slices.Contains(all.Waiting, name) {
				all.Waiting = append(all.Waiting, name)
			}
		}
	}
	return all, nil
}

// summary writes in one line what a change comes to, the feature or how many with the hosts and env they carry, and the
// secrets that wait.
func summary(verb, name string, pv Preview) string {
	line := verb + " " + name
	// Count the features where several change at once
	if pv.Features > 1 {
		line = fmt.Sprintf("%s %s", verb, words.Count(pv.Features, "feature", "features"))
	}
	line += summaryTail(pv)
	// Name the secrets still waiting for a value
	if n := len(pv.Waiting); n > 0 {
		line += ", " + words.Count(n, "secret waits", "secrets wait") + " for a value"
	}
	return line
}

// summaryTail names the hosts and env a change carries, as a sentence goes on after the feature.
func summaryTail(pv Preview) string {
	var parts []string
	// Name the hosts the change carries
	if pv.Hosts > 0 {
		parts = append(parts, words.Count(pv.Hosts, "host", "hosts"))
	}
	// Name the env values the change carries
	if pv.Env > 0 {
		parts = append(parts, words.Count(pv.Env, "env value", "env values"))
	}
	// Leave out what the change carries none of
	if len(parts) == 0 {
		return ""
	}
	return " with " + strings.Join(parts, " and ")
}

// capitalized starts a line with a capital letter.
func capitalized(line string) string {
	// Leave an empty line as it is
	if line == "" {
		return line
	}
	return strings.ToUpper(line[:1]) + line[1:]
}

// InstalledHeader labels an installed feature's row, piped and in the feature view.
var InstalledHeader = []string{"FEATURE", "TAG", "STORE", "HOSTS"}

// InstalledRow is how an installed feature shows in a row after its name, piped or in the feature view: its tag, its
// store and the hosts it needs.
func InstalledRow(f Installed, hosts []config.Host) []string {
	var needs []string
	// Take every host that names the feature among its reasons
	for _, h := range hosts {
		// Take a host the feature needs
		if slices.Contains(h.By, f.Package) {
			needs = append(needs, h.Name)
		}
	}
	return []string{f.Tag, path.Dir(f.Package), strings.Join(needs, ", ")}
}

// OptionHeader labels a feature option's row, piped and in the options view.
var OptionHeader = []string{"OPTION", "VALUE", "DEFAULT"}

// OptionRow is how a feature option shows in a row after its name, piped or in the options view: the value the
// feature sets, a dash where it sets none, and the one the option falls back to.
func OptionRow(o FeatureOption, value string) []string {
	return []string{words.Or(value, "–"), o.Default}
}
