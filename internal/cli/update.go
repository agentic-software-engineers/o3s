package cli

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/rebuild"
	"github.com/hansehart/o3s/internal/registry"
	"github.com/hansehart/o3s/internal/update"
)

// newUpdate builds the command that brings the binary, the home's stack and its features up to date, all of them or
// the ones named.
func newUpdate() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update [o3s|stack|<feature>...]",
		Short: "Bring o3s, the home's stack and its features up to date, all of them or the ones named",
		Args:  usage(cobra.ArbitraryArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			t := readTargets(args)
			out := cmd.OutOrStdout()
			// Bring the binary up first where the update takes it in
			if t.binary {
				done, err := selfUpdate(cmd.Context(), cmd.InOrStdin(), out, cmd.ErrOrStderr(), yes, t.rest())
				// Leave the home to the new binary, which carries the update on
				if err != nil || done {
					return err
				}
			}
			// End an update that takes in the binary alone
			if !t.stack && len(t.features) == 0 && !t.all {
				return nil
			}
			dir, err := workingHome()
			// Hand back a working folder that stays unknown
			if err != nil {
				return err
			}
			plan, err := update.Check(cmd.Context(), dir)
			// Hand back a home that fails to compare
			if err != nil {
				return err
			}
			plan, err = t.narrow(plan)
			// Hand back a feature the update finds nothing for
			if err != nil {
				return err
			}
			show(out, cmd.ErrOrStderr(), plan)
			// Leave a home that already holds what the binary and its stores ship
			if plan.Empty() {
				fmt.Fprintln(out, "Up to date")
				return nil
			}
			ok, err := confirm(cmd.InOrStdin(), out, isTerminal(cmd.InOrStdin()), yes)
			// Hand back an answer that fails to arrive
			if err != nil {
				return err
			}
			// Leave the home as it was when the user declines
			if !ok {
				fmt.Fprintln(out, "Left the home as it was")
				return nil
			}
			hosts, err := plan.Apply(dir)
			// Hand back an update that fails to write
			if err != nil {
				return err
			}
			// Restart the stack where the allowlist changed
			if hosts {
				// Hand back a stack that fails to restart
				if err := restartStack(cmd, dir); err != nil {
					return err
				}
			}
			cli, ok := rebuild.CLI()
			// Leave the rebuild to the user where the host holds no devcontainer CLI
			if !ok {
				fmt.Fprintln(out, "Updated")
				fmt.Fprintln(out, "Rebuild the cage to apply it")
				return nil
			}
			fmt.Fprintln(out, "Updated, rebuilding the cage")
			// Hand back a rebuild that fails, the home already updated
			if err := rebuild.Run(cmd.Context(), cli, dir, out); err != nil {
				return fmt.Errorf("updated the home, rebuild the cage to apply it: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without asking")
	return cmd
}

// targets are the parts an update brings up to date: every part where none is named, or the binary, the stack and
// the features named.
type targets struct {
	all, binary, stack bool
	features           []string
}

// readTargets reads the parts a command line names, every part where it names none.
func readTargets(args []string) targets {
	// Take every part where the command names none
	if len(args) == 0 {
		return targets{all: true, binary: true, stack: true}
	}
	var t targets
	// Sort every named part into the binary, the stack or a feature
	for _, arg := range args {
		// Take the part the word names
		switch arg {
		case "o3s":
			t.binary = true
		case "stack":
			t.stack = true
		default:
			t.features = append(t.features, arg)
		}
	}
	return t
}

// rest are the arguments that carry the update on in a new binary: the parts left once the binary is up to date,
// and none where the binary was all the update named.
func (t targets) rest() []string {
	// Carry every part on where the command named none
	if t.all {
		return []string{}
	}
	var out []string
	// Carry the stack on where the command named it
	if t.stack {
		out = append(out, "stack")
	}
	return append(out, t.features...)
}

// narrow keeps the parts of a plan the update names, and turns away a feature the plan finds nothing for.
func (t targets) narrow(p update.Plan) (update.Plan, error) {
	// Keep the whole plan where the command named no part
	if t.all {
		return p, nil
	}
	// Leave the stack out where the command named none
	if !t.stack {
		p.Images, p.Files = nil, nil
	}
	var features []update.FeatureChange
	// Keep every feature the command names
	for _, name := range t.features {
		i := slices.IndexFunc(p.Features, func(fc update.FeatureChange) bool {
			return fc.Package == name || path.Base(fc.Package) == name || packageOfKey(name) == fc.Package
		})
		// Turn away a name the update finds no feature change for
		if i < 0 {
			return update.Plan{}, fmt.Errorf("%w: %s is up to date or not installed, the names update takes are o3s, stack and the features it lists", ErrUsage, name)
		}
		features = append(features, p.Features[i])
	}
	p.Features = features
	return p, nil
}

// show prints every change an update makes, and warns on stderr about what it leaves for want of a store.
func show(out, errOut io.Writer, p update.Plan) {
	// Show every o3s image whose tag moves to this release
	for _, i := range p.Images {
		fmt.Fprintf(out, "%-14s %s -> %s\n", i.Name, i.Old, i.New)
	}
	// Head the stack's files with their folder
	if len(p.Files) > 0 {
		fmt.Fprintln(out, ".devcontainer/")
	}
	// Show every file of the stack that changes
	for _, f := range p.Files {
		mark := "~"
		// Mark a file the home lacks
		if f.New {
			mark = "+"
		}
		fmt.Fprintf(out, "  %s %s\n", mark, f.Path)
	}
	// Show every feature that changes
	for _, fc := range p.Features {
		line := fc.Package
		// Name a tag that moves
		if fc.OldTag != fc.NewTag {
			line += fmt.Sprintf("   %s -> %s", fc.OldTag, fc.NewTag)
		}
		fmt.Fprintln(out, line)
		// Show every host the feature needs anew
		for _, h := range fc.HostsAdded {
			fmt.Fprintf(out, "  + %s:%s\n", h.Name, joinPorts(registry.PortsOf(h)))
		}
		// Show every host the feature leaves
		for _, name := range fc.HostsDropped {
			fmt.Fprintf(out, "  - %s\n", name)
		}
		// Show every variable the feature brings
		for _, e := range fc.EnvAdded {
			fmt.Fprintf(out, "  + %s=%s\n", e.Name, e.Value)
		}
		// Show every variable that takes another value
		for _, e := range fc.EnvChanged {
			fmt.Fprintf(out, "  ~ %s=%s -> %s\n", e.Name, e.Old, e.New)
		}
	}
	// Name every feature its store left as it was
	for _, w := range p.Warnings {
		warn(errOut, "%s", w)
	}
}

// confirm asks whether to apply, takes --yes as the answer, and turns away a missing terminal.
func confirm(in io.Reader, out io.Writer, terminal, yes bool) (bool, error) {
	// Apply without asking where the user said so
	if yes {
		return true, nil
	}
	// Turn away a run where nobody can answer
	if !terminal {
		return false, fmt.Errorf("%w: pass --yes to apply without a terminal", ErrUsage)
	}
	fmt.Fprint(out, "Apply? [y/N] ")
	line, err := bufio.NewReader(in).ReadString('\n')
	// Read an answer that ends without a line break as it stands, and a failed read as a no
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
