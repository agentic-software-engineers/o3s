package cli

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/devcontainer"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/registry"
	"github.com/hansehart/o3s/internal/restart"
	"github.com/hansehart/o3s/internal/traffic"
	"github.com/hansehart/o3s/internal/tui"
	"github.com/hansehart/o3s/internal/update"
	"github.com/hansehart/o3s/internal/vm"
	"github.com/hansehart/o3s/internal/words"
	"github.com/hansehart/o3s/internal/workspace"
)

// runMenu shows the menu in its own screen, running each dialog's commands as this binary, until the user leaves.
func runMenu(ctx context.Context, s Streams) int {
	// Report a menu that fails to run
	if err := openMenu(ctx); err != nil {
		fmt.Fprintf(s.Err, "error: %v\n", err)
		return 1
	}
	return 0
}

// openMenu shows the menu in its own screen, opened at the view of the group a command path names, until the user
// leaves.
func openMenu(ctx context.Context, path ...string) error {
	self, err := executable()
	// Hand back a binary that fails to find itself
	if err != nil {
		return err
	}
	return tui.Run(ctx, tui.Config{
		Home:  func() menu.Home { return menuHome(ctx) },
		Start: menu.Open(path...),
		Command: func(args []string) *exec.Cmd {
			return exec.CommandContext(ctx, self, args...)
		},
	})
}

// menuLookup answers the menu's questions to the stores, reading each store once.
type menuLookup struct {
	// ctx is the run's context, which the menu's questions ask the stores within.
	ctx context.Context
	r   *registry.Resolver
}

// Catalog lists the features a store offers, with how it names them and what they do.
func (l menuLookup) Catalog(store string) ([]menu.Offer, error) {
	features, err := l.r.Features(l.ctx, store)
	// Name the store whose features stay out of reach
	if err != nil {
		return nil, fmt.Errorf("read the features of %s: %w", store, err)
	}
	out := make([]menu.Offer, len(features))
	// Take every feature's id, name and description
	for i, f := range features {
		out[i] = menu.Offer{ID: f.ID, Name: f.Name, Description: f.Description}
	}
	return out, nil
}

// Updates lists every part with a newer version, as update would bring it: o3s, the stack and each feature.
func (l menuLookup) Updates() ([]menu.Update, error) {
	var out []menu.Update
	// Name the newer release of the binary's channel
	if target, current, ok := newerRelease(l.ctx); ok {
		out = append(out, menu.Update{Target: "o3s", Name: "o3s", Change: current + " -> " + target})
	}
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return nil, err
	}
	plan, err := update.Check(l.ctx, dir)
	// Hand back a home that fails to compare
	if err != nil {
		return nil, err
	}
	// Name the stack's files and images that change
	if len(plan.Files) > 0 || len(plan.Images) > 0 {
		out = append(out, menu.Update{Target: "stack", Name: "stack", Change: words.Count(len(plan.Files), "file", "files") + ", " + words.Count(len(plan.Images), "image", "images")})
	}
	// Name every feature that moves
	for _, fc := range plan.Features {
		change := fc.OldTag + " -> " + fc.NewTag
		// Name a feature whose hosts or env change on the same tag
		if fc.OldTag == fc.NewTag {
			change = "hosts and env"
		}
		out = append(out, menu.Update{Target: fc.Package, Name: path.Base(fc.Package), Change: change})
	}
	return out, nil
}

// Add shows what adding a feature brings, as feature add prints it.
func (l menuLookup) Add(id string) (menu.Preview, error) {
	ref, err := registry.ParseRef(id)
	// Hand back an id that names no feature
	if err != nil {
		return menu.Preview{}, err
	}
	s, err := l.r.Resolve(l.ctx, ref)
	// Hand back a feature its store fails to offer
	if err != nil {
		return menu.Preview{}, err
	}
	_, store, err := loadSecrets()
	// Hand back a secrets file that fails to load
	if err != nil {
		return menu.Preview{}, err
	}
	lines, waiting := addLines(s, store)
	pv := menu.Preview{Lines: lines, Waiting: waiting, Features: 1, Hosts: len(s.Hosts), Env: len(s.Env)}
	// Offer every option the user may set, the folder o3s gives the feature left out
	for _, o := range s.Offered {
		// Take an option that is the user's to set
		if o.Name != registry.StateDir {
			pv.Options = append(pv.Options, menu.FeatureOption{Name: o.Name, Type: o.Type, Default: o.Default, Enum: o.Enum, Proposals: o.Proposals, Description: o.Description})
		}
	}
	return pv, nil
}

// Remove shows what leaves with a feature, as feature rm prints it.
func (l menuLookup) Remove(pkg string) (menu.Preview, error) {
	ref, err := registry.ParseRef(pkg)
	// Hand back an id that names no feature
	if err != nil {
		return menu.Preview{}, err
	}
	s, err := l.r.Resolve(l.ctx, ref)
	// Hand back a feature whose env stays unknown
	if err != nil {
		return menu.Preview{}, err
	}
	_, c, dc, err := loadHome()
	// Hand back a home that fails to load
	if err != nil {
		return menu.Preview{}, err
	}
	r := removal(c, dc, s)
	return menu.Preview{Lines: rmLines(pkg, r), Features: 1, Hosts: len(r.hosts), Env: len(r.env)}, nil
}

// menuHome reads what the menu offers from the home it runs in, and as much as there is outside one.
func menuHome(ctx context.Context) menu.Home {
	lookup := menuLookup{ctx: ctx, r: &registry.Resolver{}}
	now := time.Now()
	dir, err := workingHome()
	// Start without a folder where the working folder stays unknown
	if err != nil {
		return menu.Home{Lookup: lookup, Now: now}
	}
	h := menu.Home{Dir: dir, Lookup: lookup, Now: now, Traffic: map[string]menu.Traffic{}}
	c, err := config.Load(dir)
	// Take the home's hosts and stores where it holds settings
	if err == nil {
		h.Exists, h.Hosts = true, c.Hosts
		// Offer every store by its id
		for _, s := range c.Stores {
			h.Stores = append(h.Stores, s.ID)
		}
	}
	dc, err := devcontainer.Load(dir)
	// Offer every feature the home installs from a store, in the order the cage installs them
	if err == nil {
		h.Installed = installed(dc)
	}
	record, running, err := traffic.Read(ctx, dir)
	h.Running = running
	// Show what the gateway recorded where its record reads
	if err == nil {
		// Show the data each host moved and when
		for name, t := range record.Hosts {
			h.Traffic[name] = hostTraffic(t, now)
		}
		// Offer each refused name, the most recent first, with how often and when
		for _, b := range record.Refused() {
			h.Blocked = append(h.Blocked, menu.Option{Value: b.Name, Columns: []string{words.Count(b.Attempts, "attempt", "attempts"), traffic.Ago(b.LastSeen, now)}})
		}
	}
	h.Stack = stackFacts(ctx, dir, h.Exists)
	// Show where the VM of a home that lives in one stands, in place of the stack it runs inside
	if m, inVM, err := homeMachine(); inVM {
		h.VM = vm.Absent
		// Read the VM's state where its manager answers
		if err == nil {
			h.VM, _ = m.state(ctx)
		}
		h.Stack = []menu.Fact{{Label: "VM", Value: string(h.VM)}}
	}
	ws, err := workspace.Load(home.Workspace(dir))
	// Offer every project the workspace lists
	if err == nil {
		// Offer each project in the short form rm takes
		for _, r := range projects(ws) {
			h.Projects = append(h.Projects, r.Short())
		}
	}
	h.Secrets = menuSecrets(h.Hosts)
	return h
}

// installed lists the features the config installs from a store, in the order the cage installs them, each with its
// tag and the options it sets.
func installed(dc *devcontainer.Config) []menu.Installed {
	var out []menu.Installed
	// Take every feature that names a store
	for _, key := range inInstallOrder(dc) {
		ref, err := registry.ParseRef(key)
		// Leave out a feature no store offers
		if err != nil {
			continue
		}
		f := menu.Installed{Package: ref.Package(), Tag: words.Or(ref.Tag, "latest"), Options: map[string]string{}}
		// Take every option the feature sets, the folder o3s gives it left out
		for name, v := range dc.FeatureOptions(ref.Package()) {
			// Take an option that is the user's to set
			if name != registry.StateDir {
				f.Options[name] = fmt.Sprint(v)
			}
		}
		out = append(out, f)
	}
	return out
}

// stackFacts names whether each part of the stack runs, and nothing outside a home.
func stackFacts(ctx context.Context, dir string, exists bool) []menu.Fact {
	// Name nothing outside a home
	if !exists {
		return nil
	}
	var facts []menu.Fact
	// Name the state of every part
	for _, part := range []string{"cage", "gateway", "proxy"} {
		ids, err := restart.Containers(ctx, dir, part)
		state := "stopped"
		// Name a part whose state stays unknown
		if err != nil {
			state = "unknown"
		}
		// Name a part that runs
		if len(ids) > 0 {
			state = "running"
		}
		facts = append(facts, menu.Fact{Label: strings.ToUpper(part[:1]) + part[1:], Value: state})
	}
	return facts
}

// menuSecrets lists every secret the hosts carry or the secrets file names, once each, noting whether it holds a
// value.
func menuSecrets(hosts []config.Host) []menu.Secret {
	var names []string
	// Take every secret a host carries
	for _, host := range hosts {
		// Take a host's secret once
		if host.Secret != "" {
			names = append(names, host.Secret)
		}
	}
	_, store, err := loadSecrets()
	// Take every secret the secrets file names where it reads
	if err == nil {
		names = append(names, store.Names()...)
	}
	slices.Sort(names)
	var out []menu.Secret
	// Offer every secret once, noting whether it holds a value
	for _, name := range slices.Compact(names) {
		_, set := store.Value(name)
		out = append(out, menu.Secret{Name: name, Set: set})
	}
	return out
}

// isInteractive reports whether both ends of a stream reach a terminal, as a menu needs.
func isInteractive(in io.Reader, out io.Writer) bool {
	return isTerminal(in) && isTerminal(out)
}
