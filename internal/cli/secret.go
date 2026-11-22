package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/secrets"
	"github.com/hansehart/o3s/internal/words"
)

// newSecret builds the commands that show, store and remove the values the proxy injects.
func newSecret() *cobra.Command {
	s := group("secret", "Show, store or remove the values the proxy injects", showSecrets)
	s.AddCommand(newSecretAdd(), newSecretRm())
	return s
}

// newSecretAdd builds the command that stores a secret's value, read from stdin.
func newSecretAdd() *cobra.Command {
	var noBindHint bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Store a secret's value, read from stdin",
		Args:  usage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			in := cmd.InOrStdin()
			// Turn away a value typed at a terminal, so it stays out of the screen and the history
			if isTerminal(in) {
				return fmt.Errorf("%w: pipe the value on stdin, as in: echo \"$TOKEN\" | o3s secret add %s", ErrUsage, name)
			}
			value, err := io.ReadAll(in)
			// Hand back a stdin that fails to read
			if err != nil {
				return err
			}
			path, store, err := loadSecrets()
			// Hand back a secrets file that fails to load
			if err != nil {
				return err
			}
			replaced, err := store.Set(name, string(value))
			// Hand back a value the proxy would refuse
			if err != nil {
				return err
			}
			// Hand back a secrets file that fails to save
			if err := store.Save(path); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			stored := "Stored"
			// Name a value that replaced the one the secret held, keeping both values hidden
			if replaced {
				stored = "Replaced"
			}
			fmt.Fprintf(out, "%s %s\n", stored, name)
			// Point a host-free name at the command that binds it, where a feature leaves the binding to the user
			if !noBindHint && len(hostsNaming(name)) == 0 {
				fmt.Fprintf(out, "Bind it to a host with: o3s firewall add <host> --secret %s\n", name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noBindHint, "no-bind-hint", false, "leave out the hint for a secret a feature binds next")
	_ = cmd.Flags().MarkHidden("no-bind-hint")
	return cmd
}

// newSecretRm builds the command that removes secrets' values, once the user agrees.
func newSecretRm() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm <name>...",
		Short: "Remove secrets' values",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, store, err := loadSecrets()
			// Hand back a secrets file that fails to load
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			// Name every value that leaves and the hosts that stop receiving it
			for _, name := range args {
				// Turn away a name the file lacks
				if _, ok := store.Value(name); !ok {
					return fmt.Errorf("%w %s", secrets.ErrUnknown, name)
				}
				line := "  - " + name
				// Name the hosts that stop receiving the value
				if hosts := hostsNaming(name); len(hosts) > 0 {
					line += ", " + words.JoinAnd(hosts) + " stop receiving it"
				}
				fmt.Fprintln(out, line)
			}
			ok, err := confirm(cmd.InOrStdin(), out, isTerminal(cmd.InOrStdin()), yes, "remove")
			// Hand back an answer that fails to arrive
			if err != nil {
				return err
			}
			// Leave the secrets as they were when the user declines
			if !ok {
				fmt.Fprintln(out, "Left the secrets as they were")
				return nil
			}
			// Remove every value the command names
			for _, name := range args {
				store.Remove(name)
			}
			// Hand back a secrets file that fails to save
			if err := store.Save(path); err != nil {
				return err
			}
			// Name every value that left
			for _, name := range args {
				fmt.Fprintf(out, "Removed %s\n", name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove without asking")
	return cmd
}

// showSecrets writes every secret, whether it holds a value, and the hosts that carry it.
func showSecrets(cmd *cobra.Command) error {
	_, store, err := loadSecrets()
	// Hand back a secrets file that fails to load
	if err != nil {
		return err
	}
	names := store.Names()
	c, _ := homeConfig()
	// Add the names the home's hosts carry, stored or missing
	for _, h := range c.Hosts {
		// Add a name the file is yet to hold
		if h.Secret != "" && !slices.Contains(names, h.Secret) {
			names = append(names, h.Secret)
		}
	}
	slices.Sort(names)
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(menu.SecretHeader, "\t"))
	// Write a row per name
	for _, name := range names {
		_, set := store.Value(name)
		fmt.Fprintln(w, strings.Join(append([]string{name}, menu.SecretRow(set, hostsNaming(name))...), "\t"))
	}
	return w.Flush()
}

// loadSecrets finds and reads the secrets file the stack mounts into the proxy.
func loadSecrets() (string, secrets.Store, error) {
	path, err := secrets.Path()
	// Hand back a secrets file whose place stays unknown
	if err != nil {
		return "", secrets.Store{}, err
	}
	store, err := secrets.Load(path)
	return path, store, err
}

// homeConfig reads the settings of the home the command runs in.
func homeConfig() (config.Config, error) {
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return config.Config{}, err
	}
	return config.Load(dir)
}

// hostsNaming lists the hosts of the home that carry a secret, and an empty list outside a home.
func hostsNaming(name string) []string {
	c, err := homeConfig()
	// Hand back an empty host list outside a home
	if err != nil && errors.Is(err, config.ErrNoHome) {
		return nil
	}
	var hosts []string
	// Collect every host that carries the name
	for _, h := range c.Hosts {
		// Keep a host that carries the name
		if h.Secret == name {
			hosts = append(hosts, h.Name)
		}
	}
	return hosts
}

// isTerminal reports whether a stream reaches a terminal.
func isTerminal(stream any) bool {
	f, ok := stream.(*os.File)
	// Read anything but a file as a pipe
	if !ok {
		return false
	}
	return term.IsTerminal(f.Fd())
}
