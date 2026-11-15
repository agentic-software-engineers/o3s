package cli

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"slices"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/devcontainer"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/registry"
	"github.com/hansehart/o3s/internal/tui"
	"github.com/hansehart/o3s/internal/workspace"
)

// runMenu shows the menu in its own screen, running each dialog's commands as this binary, until the user leaves.
func runMenu(ctx context.Context, s Streams) int {
	self, err := executable()
	// Report a binary that fails to find itself
	if err != nil {
		fmt.Fprintf(s.Err, "error: %v\n", err)
		return 1
	}
	err = tui.Run(ctx, tui.Config{
		Home: func() menu.Home { return menuHome(ctx) },
		Command: func(args []string) *exec.Cmd {
			return exec.CommandContext(ctx, self, args...)
		},
	})
	// Report a menu that fails to run
	if err != nil {
		fmt.Fprintf(s.Err, "error: %v\n", err)
		return 1
	}
	return 0
}

// menuLookup answers the menu's questions to the stores, reading each store once.
type menuLookup struct {
	// ctx is the run's context, which the menu's questions ask the stores within.
	ctx context.Context
	r   *registry.Resolver
}

// Features lists the ids of the features a store offers.
func (l menuLookup) Features(store string) ([]string, error) {
	features, err := l.r.Features(l.ctx, store)
	// Name the store whose features stay out of reach
	if err != nil {
		return nil, fmt.Errorf("read the features of %s: %w", store, err)
	}
	return ids(features), nil
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
	return menu.Preview{Lines: lines, Waiting: waiting}, nil
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
	return menu.Preview{Lines: rmLines(pkg, removal(c, dc, s))}, nil
}

// menuHome reads what the menu offers from the home it runs in, and as much as there is outside one.
func menuHome(ctx context.Context) menu.Home {
	lookup := menuLookup{ctx: ctx, r: &registry.Resolver{}}
	dir, err := workingHome()
	// Start without a folder where the working folder stays unknown
	if err != nil {
		return menu.Home{Lookup: lookup}
	}
	h := menu.Home{Dir: dir, Lookup: lookup}
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
	// Offer every feature the home installs from a store
	if err == nil {
		// Take every feature that names a store
		for _, key := range dc.Features() {
			// Offer the feature by its package
			if ref, err := registry.ParseRef(key); err == nil {
				h.Features = append(h.Features, ref.Package())
			}
		}
	}
	ws, err := workspace.Load(home.Workspace(dir))
	// Offer every project the workspace lists
	if err == nil {
		// Offer each project in the short form rm takes
		for _, r := range projects(ws) {
			h.Projects = append(h.Projects, r.Short())
		}
	}
	var names []string
	// Take every secret a host carries
	for _, host := range h.Hosts {
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
	// Offer every secret once, noting whether it holds a value
	for _, name := range slices.Compact(names) {
		_, set := store.Value(name)
		h.Secrets = append(h.Secrets, menu.Secret{Name: name, Set: set})
	}
	return h
}

// isInteractive reports whether both ends of a stream reach a terminal, as a menu needs.
func isInteractive(in io.Reader, out io.Writer) bool {
	return isTerminal(in) && isTerminal(out)
}
