package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/preview"
)

// newRoot builds the o3s command tree, wired to the given streams.
func newRoot(s Streams) *cobra.Command {
	root := &cobra.Command{
		Use:           "o3s",
		Short:         "Set up the cage an o3s home runs",
		Version:       version(),
		Args:          usage(cobra.NoArgs),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	var previews bool
	root.PersistentFlags().BoolVar(&previews, "preview", false, "turn on the features o3s ships as previews, as O3S_PREVIEW=1 does")
	root.PersistentPreRun = func(*cobra.Command, []string) {
		// Turn every preview on for this run, where the flag asks
		if previews {
			preview.EnableAll()
		}
	}
	root.SetIn(s.In)
	root.SetOut(s.Out)
	root.SetErr(s.Err)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	})
	root.AddCommand(newInit(), newFirewall(), newSecret(), newFeature(), newProject(), newCage(), newVM(), newUpdate())
	return root
}

// usage marks every rejected argument list as a usage error.
func usage(args cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, a []string) error {
		// Mark the rejection as a usage error
		if err := args(cmd, a); err != nil {
			return fmt.Errorf("%w: %w", ErrUsage, err)
		}
		return nil
	}
}

// group builds a command whose own name shows what it holds: its view at a terminal, and plain tables
// where its output goes on to a pipe, a file or a flag asks for another form.
func group(use, short string, show func(cmd *cobra.Command) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Open the group's view for a user at a terminal who asks for no other form with a flag of its own
			if cmd.LocalNonPersistentFlags().NFlag() == 0 && isInteractive(cmd.InOrStdin(), cmd.OutOrStdout()) {
				return openMenu(cmd.Context(), strings.Fields(cmd.CommandPath())[1:]...)
			}
			return show(cmd)
		},
	}
}

// workingHome finds the o3s workingHome a command works on, the folder it runs in.
func workingHome() (string, error) {
	return os.Getwd()
}
