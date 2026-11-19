package cli

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/registry"
)

// newStore builds the commands that show, add and remove the stores features come from.
func newStore() *cobra.Command {
	s := group("store", "Show, add or remove the stores features come from", showStores)
	s.AddCommand(newStoreAdd(), newStoreRm())
	return s
}

// newStoreAdd builds the command that adds stores once their collections prove them real.
func newStoreAdd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <store>...",
		Short: "Add stores and show the features they offer",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			offered := map[string][]registry.Feature{}
			_, err := edit(func(c *config.Config) error {
				// Add every store, all of them or none
				for _, id := range args {
					// Turn away a store id that breaks a rule
					if _, err := c.AddStore(id); err != nil {
						return err
					}
					features, err := registry.Collection(cmd.Context(), id)
					// Name the store whose features stay out of reach
					if err != nil {
						return fmt.Errorf("read the features of %s: %w", id, err)
					}
					offered[id] = features
				}
				return nil
			})
			// Hand back a store that fails to join the list
			if err != nil {
				return err
			}
			// Show every store with the features it offers
			for _, id := range args {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n  + %s\nAdded %s\n", id, strings.Join(ids(offered[id]), ", "), id)
			}
			return nil
		},
	}
}

// newStoreRm builds the command that removes stores from the list, once the user agrees.
func newStoreRm() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm <store>...",
		Short: "Remove stores",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			// Name every store that leaves, the features installed from it staying
			for _, id := range args {
				fmt.Fprintf(out, "  - %s\n", id)
			}
			ok, err := confirm(cmd.InOrStdin(), out, isTerminal(cmd.InOrStdin()), yes, "remove")
			// Hand back an answer that fails to arrive
			if err != nil {
				return err
			}
			// Leave the stores as they were when the user declines
			if !ok {
				fmt.Fprintln(out, "Left the stores as they were")
				return nil
			}
			_, err = edit(func(c *config.Config) error {
				// Remove every store, all of them or none
				for _, id := range args {
					// Hand back a store the list lacks
					if err := c.RemoveStore(id); err != nil {
						return err
					}
				}
				return nil
			})
			// Hand back a store that stays on the list
			if err != nil {
				return err
			}
			// Name every store that left
			for _, id := range args {
				fmt.Fprintf(out, "Removed %s\n", id)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove without asking")
	return cmd
}

// showStores writes every store with the number of features it offers.
func showStores(cmd *cobra.Command) error {
	c, err := homeConfig()
	// Hand back a home that fails to load
	if err != nil {
		return err
	}
	counts := make([]string, len(c.Stores))
	var wg sync.WaitGroup
	// Ask every store for its collection at once
	for i, s := range c.Stores {
		wg.Go(func() {
			features, err := registry.Collection(cmd.Context(), s.ID)
			counts[i] = strconv.Itoa(len(features))
			// Mark a store out of reach
			if err != nil {
				counts[i] = "!"
			}
		})
	}
	wg.Wait()
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STORE\tFEATURES")
	// Write one row per store
	for i, s := range c.Stores {
		fmt.Fprintf(w, "%s\t%s\n", s.ID, counts[i])
	}
	return w.Flush()
}

// ids lists the ids of features.
func ids(features []registry.Feature) []string {
	out := make([]string, len(features))
	// Take every feature's id
	for i, f := range features {
		out[i] = f.ID
	}
	return out
}
