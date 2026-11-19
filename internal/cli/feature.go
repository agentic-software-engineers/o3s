package cli

import (
	"fmt"
	"io"
	"maps"
	"path"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/devcontainer"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/registry"
	"github.com/hansehart/o3s/internal/secrets"
	"github.com/hansehart/o3s/internal/words"
)

// newFeature builds the commands that show, add, configure, order and remove the features the cage installs,
// and the stores they come from.
func newFeature() *cobra.Command {
	f := group("feature", "Show, add, configure, order or remove the features the cage installs", showFeatures)
	f.AddCommand(newFeatureAdd(), newFeatureConfigure(), newFeatureOrder(), newFeatureRm(), newStore())
	return f
}

// newFeatureAdd builds the command that adds features with the hosts and env they need, saved together.
func newFeatureAdd() *cobra.Command {
	var noPreview bool
	var options []string
	cmd := &cobra.Command{
		Use:   "add <id>...",
		Short: "Add features with the hosts and env they need",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			setups, err := resolveAll(cmd, args)
			// Hand back a feature its store fails to offer
			if err != nil {
				return err
			}
			// Hand back an option no feature here takes as given
			if err := applyOptions(setups, options); err != nil {
				return err
			}
			dir, c, dc, err := loadHome()
			// Hand back a home that fails to load
			if err != nil {
				return err
			}
			hosts := false
			// Apply every feature before anything is saved
			for _, s := range setups {
				pkg := s.Ref.Package()
				// Open every host the feature needs
				for _, h := range s.Hosts {
					// Name the host that breaks a rule, the files left as they were
					if err := c.Need(pkg, config.Change{Name: h.Name, Ports: h.Ports, Secret: h.Secret, Inject: h.Inject}); err != nil {
						return fmt.Errorf("%s needs %s: %s", pkg, h.Name, err.Error())
					}
				}
				hosts = hosts || len(s.Hosts) > 0
				dc.SetFeature(pkg, s.Ref.String(), s.Options)
				// Set every variable the feature brings
				for _, e := range s.Env {
					dc.SetEnv(e.Name, e.Value)
				}
			}
			// Hand back an install order the features break
			if err := reorder(cmd, dc, nil); err != nil {
				return err
			}
			// Hand back a home that fails to save
			if err := saveHome(dir, c, dc); err != nil {
				return err
			}
			_, store, err := loadSecrets()
			// Hand back a secrets file that fails to load
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			var waiting []string
			// Show what each feature brings, and collect the secrets they wait for
			for _, s := range setups {
				lines, wait := addLines(s, store)
				// Show the change, unless the menu showed it already
				if !noPreview {
					// Print every line of the change
					for _, line := range lines {
						fmt.Fprintln(out, line)
					}
				}
				fmt.Fprintf(out, "Added %s\n", s.Ref.Package())
				// Name each waiting secret once
				for _, name := range wait {
					// Keep a secret named already out of the list
					if !slices.Contains(waiting, name) {
						waiting = append(waiting, name)
					}
				}
			}
			// Name how to give each waiting secret its value
			for _, name := range waiting {
				warn(cmd.ErrOrStderr(), "%s waits for its value: echo \"$TOKEN\" | o3s secret add %s", name, name)
			}
			// Restart the stack where the allowlist changed
			if hosts {
				// Hand back a stack that fails to restart
				if err := restartStack(cmd, dir); err != nil {
					return err
				}
			}
			// Name the rebuild that installs the feature, or the features
			if len(setups) == 1 {
				fmt.Fprintln(out, "Rebuild the cage to install it")
				return nil
			}
			fmt.Fprintln(out, "Rebuild the cage to install them")
			return nil
		},
	}
	previewFlag(cmd, &noPreview)
	cmd.Flags().StringArrayVar(&options, "option", nil, "set an option the feature offers, as name=value or id:name=value, repeatable")
	return cmd
}

// resolveAll resolves every feature id against its store, before any of them changes the home.
func resolveAll(cmd *cobra.Command, ids []string) ([]registry.Setup, error) {
	var r registry.Resolver
	setups := make([]registry.Setup, 0, len(ids))
	// Resolve every id in the order given
	for _, id := range ids {
		ref, err := registry.ParseRef(id)
		// Hand back an id that names no feature
		if err != nil {
			return nil, err
		}
		s, err := r.Resolve(cmd.Context(), ref)
		// Hand back a feature its store fails to offer
		if err != nil {
			return nil, err
		}
		setups = append(setups, s)
	}
	return setups, nil
}

// newFeatureRm builds the command that removes features with their env and the hosts they alone need, once the
// user agrees.
func newFeatureRm() *cobra.Command {
	var noPreview, yes bool
	cmd := &cobra.Command{
		Use:   "rm <id>...",
		Short: "Remove features with their env and the hosts they alone need",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, c, dc, err := loadHome()
			// Hand back a home that fails to load
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			var r registry.Resolver
			var pkgs []string
			var all removalOf
			// Take every feature off the home, collecting what leaves with each
			for _, id := range args {
				ref, err := registry.ParseRef(id)
				// Hand back an id that names no feature
				if err != nil {
					return err
				}
				pkg := ref.Package()
				// Turn away a feature the home lacks
				if !dc.RemoveFeature(pkg) {
					return fmt.Errorf("%w %s", registry.ErrUnknownFeature, pkg)
				}
				s, err := r.Resolve(cmd.Context(), ref)
				// Hand back a feature whose env stays unknown, the files left as they were
				if err != nil {
					return err
				}
				gone := removal(c, dc, s)
				c.Release(pkg)
				// Drop every variable the feature set
				for _, e := range gone.env {
					dc.RemoveEnv(e.Name)
				}
				// Show what leaves with the feature, unless the menu showed it already
				if !noPreview {
					// Print every line of the change
					for _, line := range rmLines(pkg, gone) {
						fmt.Fprintln(out, line)
					}
				}
				pkgs = append(pkgs, pkg)
				all.hosts, all.env = append(all.hosts, gone.hosts...), append(all.env, gone.env...)
			}
			ok, err := confirm(cmd.InOrStdin(), out, isTerminal(cmd.InOrStdin()), yes, "remove")
			// Hand back an answer that fails to arrive
			if err != nil {
				return err
			}
			// Leave the home as it was when the user declines
			if !ok {
				fmt.Fprintln(out, "Left the features as they were")
				return nil
			}
			// Hand back an install order the remaining features break
			if err := reorder(cmd, dc, nil); err != nil {
				return err
			}
			// Hand back a home that fails to save
			if err := saveHome(dir, c, dc); err != nil {
				return err
			}
			// Name every feature that left
			for _, pkg := range pkgs {
				fmt.Fprintf(out, "Removed %s\n", pkg)
			}
			// Restart the stack where the allowlist changed
			if len(all.hosts) > 0 {
				return restartStack(cmd, dir)
			}
			return nil
		},
	}
	previewFlag(cmd, &noPreview)
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove without asking")
	return cmd
}

// newFeatureConfigure builds the command that shows or sets the options of an installed feature.
func newFeatureConfigure() *cobra.Command {
	var options []string
	cmd := &cobra.Command{
		Use:   "configure <id>",
		Short: "Show or set the options of an installed feature",
		Args:  usage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			// Hand back an id that names no feature
			if err != nil {
				return err
			}
			dir, c, dc, err := loadHome()
			// Hand back a home that fails to load
			if err != nil {
				return err
			}
			key, ok := installedKey(dc, ref.Package())
			// Turn away a feature the home lacks
			if !ok {
				return fmt.Errorf("%w %s", registry.ErrUnknownFeature, ref.Package())
			}
			installed, _ := registry.ParseRef(key)
			s, err := new(registry.Resolver).Resolve(cmd.Context(), installed)
			// Hand back a feature its store fails to offer
			if err != nil {
				return err
			}
			had := dc.FeatureOptions(ref.Package())
			out := cmd.OutOrStdout()
			// Show the options and the values they hold where the command sets none
			if len(options) == 0 {
				return showOptions(out, s.Offered, had)
			}
			setups := []registry.Setup{s}
			// Hand back an option the feature takes none of as given
			if err := applyOptions(setups, options); err != nil {
				return err
			}
			dc.SetFeature(ref.Package(), key, setups[0].Options)
			// Hand back a home that fails to save
			if err := saveHome(dir, c, dc); err != nil {
				return err
			}
			fmt.Fprintln(out, ref.Package())
			// Show every option the command set, with its value before and after
			for _, name := range slices.Sorted(maps.Keys(setups[0].Options)) {
				// Leave out the folder o3s sets itself
				if name == registry.StateDir {
					continue
				}
				fmt.Fprintf(out, "  ~ %s %s -> %v\n", name, valueOrDefault(had, name, s.Offered), setups[0].Options[name])
			}
			fmt.Fprintln(out, "Rebuild the cage to apply it")
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&options, "option", nil, "set an option the feature offers, as name=value, repeatable")
	return cmd
}

// newFeatureOrder builds the command that puts features first in the order the cage installs them.
func newFeatureOrder() *cobra.Command {
	return &cobra.Command{
		Use:   "order <id>...",
		Short: "Install these features first, in this order, and the rest after them",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			var first []string
			// Read every feature the command puts first
			for _, id := range args {
				ref, err := registry.ParseRef(id)
				// Hand back an id that names no feature
				if err != nil {
					return err
				}
				first = append(first, ref.Package())
			}
			dir, c, dc, err := loadHome()
			// Hand back a home that fails to load
			if err != nil {
				return err
			}
			// Hand back an order the features break
			if err := reorder(cmd, dc, first); err != nil {
				return err
			}
			// Hand back a home that fails to save
			if err := saveHome(dir, c, dc); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			// Show the order the cage installs the features in
			for i, pkg := range dc.InstallOrder() {
				fmt.Fprintf(out, "%d. %s\n", i+1, pkg)
			}
			fmt.Fprintln(out, "Rebuild the cage to apply it")
			return nil
		},
	}
}

// showFeatures writes every feature in the order the cage installs them, with its tag and the hosts it needs.
func showFeatures(cmd *cobra.Command) error {
	_, c, dc, err := loadHome()
	// Hand back a home that fails to load
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(menu.InstalledHeader, "\t"))
	// Write one row per feature, in the order the cage installs them
	for _, f := range installed(dc) {
		fmt.Fprintln(w, strings.Join(append([]string{path.Base(f.Package)}, menu.InstalledRow(f, c.Hosts)...), "\t"))
	}
	return w.Flush()
}

// inInstallOrder lists the config's features in the order the cage installs them: the order the config fixes,
// then any it leaves out, as the config holds them.
func inInstallOrder(dc *devcontainer.Config) []string {
	keys := dc.Features()
	var out []string
	// Take the features the order names, in its order
	for _, pkg := range dc.InstallOrder() {
		// Take the feature of that package, whatever its tag
		if i := slices.IndexFunc(keys, func(k string) bool { return packageOfKey(k) == pkg }); i >= 0 {
			out = append(out, keys[i])
		}
	}
	// Take every feature the order leaves out
	for _, k := range keys {
		// Take a feature not taken yet
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// reorder fixes the order the cage installs the config's features in, the ones named first in front and every
// feature behind the ones it installs after, as their stores tell.
func reorder(cmd *cobra.Command, dc *devcontainer.Config, first []string) error {
	var r registry.Resolver
	var features []string
	after := map[string][]string{}
	// Read what every installed feature installs after, from the order it stands in
	for _, key := range inInstallOrder(dc) {
		ref, err := registry.ParseRef(key)
		// Leave a feature no store offers out of the order
		if err != nil {
			continue
		}
		features = append(features, ref.Package())
		// Take what the feature follows where its store answers
		if s, err := r.Resolve(cmd.Context(), ref); err == nil {
			after[ref.Package()] = s.After
		}
	}
	order, err := devcontainer.Ordered(features, after, first)
	// Hand back an order the features break
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}
	dc.SetInstallOrder(order)
	return nil
}

// installedKey finds the key a feature of a package holds in the config, and whether it holds one.
func installedKey(dc *devcontainer.Config, pkg string) (string, bool) {
	i := slices.IndexFunc(dc.Features(), func(k string) bool { return packageOfKey(k) == pkg })
	// Report a package the config lacks
	if i < 0 {
		return "", false
	}
	return dc.Features()[i], true
}

// packageOfKey reads the package of a feature key, and the key itself where no store offers it.
func packageOfKey(key string) string {
	// Take the package of a feature a store offers
	if ref, err := registry.ParseRef(key); err == nil {
		return ref.Package()
	}
	return key
}

// showOptions writes the options a feature offers, the value each holds and the one it falls back to.
func showOptions(out io.Writer, offered []registry.Option, had map[string]any) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(menu.OptionHeader, "\t"))
	// Write a row per option the user may set
	for _, o := range userOptions(offered) {
		value := ""
		// Show the value the config gives the option
		if v, ok := had[o.Name]; ok {
			value = fmt.Sprint(v)
		}
		fmt.Fprintln(w, strings.Join(append([]string{o.Name}, menu.OptionRow(o, value)...), "\t"))
	}
	return w.Flush()
}

// valueOrDefault writes the value an option holds, or the one it falls back to where the config gives none.
func valueOrDefault(had map[string]any, name string, offered []registry.Option) string {
	// Take the value the config gives the option
	if v, ok := had[name]; ok {
		return fmt.Sprint(v)
	}
	// Take the option's own fallback
	if i := slices.IndexFunc(offered, func(o registry.Option) bool { return o.Name == name }); i >= 0 {
		return words.Or(offered[i].Default, "none")
	}
	return "none"
}

// addLines shows what a feature brings, and lists the secrets it waits for.
func addLines(s registry.Setup, store secrets.Store) ([]string, []string) {
	lines := []string{s.Ref.String()}
	var waiting []string
	// Show every host the feature opens, and the secret it carries
	for _, h := range s.Hosts {
		line := fmt.Sprintf("  + %s:%s", h.Name, config.JoinPorts(registry.PortsOf(h)))
		// Name the secret, and warn while it waits for its value
		if h.Secret != "" {
			line += "  " + h.Secret
			// Warn about a secret without a value
			if _, ok := store.Value(h.Secret); !ok {
				line += " !"
				waiting = append(waiting, h.Secret)
			}
		}
		lines = append(lines, line)
	}
	// Show every variable the feature sets
	for _, e := range s.Env {
		lines = append(lines, fmt.Sprintf("  + %s=%s", e.Name, e.Value))
	}
	return lines, waiting
}

// removalOf is what removing a feature takes from a home: the hosts nothing else needs and the env it set.
type removalOf struct {
	hosts []string
	env   []registry.Env
}

// removal works out what removing a feature takes from a home, leaving the home as it is.
func removal(c config.Config, dc *devcontainer.Config, s registry.Setup) removalOf {
	ports := map[string][]int{}
	// Remember every host's ports for the hosts the removal drops
	for _, h := range c.Hosts {
		ports[h.Name] = h.Ports
	}
	after := c
	after.Hosts = slices.Clone(c.Hosts)
	var r removalOf
	// Name every host nothing else needs, with its ports
	for _, name := range after.Release(s.Ref.Package()) {
		r.hosts = append(r.hosts, name+":"+config.JoinPorts(ports[name]))
	}
	// Take every variable of the feature the config sets
	for _, e := range s.Env {
		// Take a variable the config holds
		if _, ok := dc.Env(e.Name); ok {
			r.env = append(r.env, e)
		}
	}
	return r
}

// rmLines shows what leaves with a feature.
func rmLines(pkg string, r removalOf) []string {
	lines := []string{pkg}
	// Show every host nothing needs any more
	for _, h := range r.hosts {
		lines = append(lines, "  - "+h)
	}
	// Show every variable the removal drops
	for _, e := range r.env {
		lines = append(lines, fmt.Sprintf("  - %s=%s", e.Name, e.Value))
	}
	return lines
}

// previewFlag adds the hidden flag the menu passes once it has shown a feature's change.
func previewFlag(cmd *cobra.Command, noPreview *bool) {
	cmd.Flags().BoolVar(noPreview, "no-preview", false, "leave out the change the menu showed already")
	_ = cmd.Flags().MarkHidden("no-preview")
}

// loadHome reads the settings and the dev container config of the home the command runs in.
func loadHome() (string, config.Config, *devcontainer.Config, error) {
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return "", config.Config{}, nil, err
	}
	c, err := config.Load(dir)
	// Hand back settings that fail to load
	if err != nil {
		return "", config.Config{}, nil, err
	}
	dc, err := devcontainer.Load(dir)
	return dir, c, dc, err
}

// saveHome writes the dev container config and then the settings of a home.
func saveHome(dir string, c config.Config, dc *devcontainer.Config) error {
	// Hand back a dev container config that fails to save
	if err := dc.Save(dir); err != nil {
		return err
	}
	return c.Save(dir)
}
