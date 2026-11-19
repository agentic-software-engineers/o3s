package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/restart"
	"github.com/hansehart/o3s/internal/traffic"
	"github.com/hansehart/o3s/internal/words"
)

// newFirewall builds the commands that show, allow and remove hosts.
func newFirewall() *cobra.Command {
	var limit int
	var asJSON bool
	fw := group("firewall", "Show, allow or remove the hosts the cage may reach", func(cmd *cobra.Command) error {
		return showFirewall(cmd, limit, asJSON)
	})
	fw.Flags().IntVar(&limit, "limit", 0, "show up to this many blocked names, 0 for all")
	fw.Flags().BoolVar(&asJSON, "json", false, "write the allowed hosts and the blocked names as JSON")
	fw.AddCommand(newFirewallAdd(), newFirewallRm(), newFirewallPrune())
	return fw
}

// newFirewallAdd builds the command that allows hosts, or updates the ones it names.
func newFirewallAdd() *cobra.Command {
	var ch config.Change
	var forValue string
	var yes bool
	cmd := &cobra.Command{
		Use:   "add <host>...",
		Short: "Allow hosts, or update them",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			now := time.Now()
			// Give the hosts the end time their duration reaches
			if forValue != "" {
				d, err := parseFor(forValue)
				// Hand back a duration that reads as no time
				if err != nil {
					return err
				}
				ch.Expires = config.EndTime(now.Add(d))
			}
			c, err := homeConfig()
			// Hand back a home that fails to load
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			narrowed := false
			// Name everyone each host reaches that would close sooner than it does now
			for _, name := range args {
				// Name the consequences for a host the change closes sooner
				if h, ok := c.Find(name); ok && narrows(h.Expires, ch.Expires) {
					consequences(out, h, forValue, ch.Expires, now)
					narrowed = true
				}
			}
			// Ask before any host closes sooner than it would
			if narrowed {
				ok, err := confirm(cmd.InOrStdin(), out, isTerminal(cmd.InOrStdin()), yes)
				// Hand back an answer that fails to arrive
				if err != nil {
					return err
				}
				// Leave the allowlist as it was when the user declines
				if !ok {
					fmt.Fprintln(out, "Left the allowlist as it was")
					return nil
				}
			}
			return change(cmd, func(c *config.Config) error {
				// Allow every host the command names, all of them or none
				for _, name := range args {
					one := ch
					one.Name = name
					diffs, err := c.Allow(one)
					// Hand back a change the allowlist turns away
					if err != nil {
						return err
					}
					// List the fields an update changed, under the host they belong to
					if len(diffs) > 0 {
						fmt.Fprintln(out, name)
						// Show every changed field with its value before and after
						for _, d := range diffs {
							// Show end times as the local clock time they close at
							if d.Field == "expires" {
								d.Old, d.New = config.ClosesAtOrNone(d.Old, now), config.ClosesAt(d.New, now)
							}
							fmt.Fprintf(out, "  ~ %s %s -> %s\n", d.Field, words.Or(d.Old, "none"), words.Or(d.New, "none"))
						}
					}
					h, _ := c.Find(name)
					fmt.Fprintln(out, allowed(h, now))
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&forValue, "for", "", "keep the hosts open this long, as seconds or a duration like 20m or 1h (default for good)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "close a host sooner without asking")
	cmd.Flags().IntSliceVarP(&ch.Ports, "port", "p", nil, "a port to allow, 1-65535, repeatable (default 443)")
	cmd.Flags().StringVar(&ch.Secret, "secret", "", "the name of the secret the proxy injects for these hosts")
	cmd.Flags().StringSliceVar(&ch.Inject, "inject", nil, "where the secret goes: header, query or body, repeatable (default header)")
	return cmd
}

// newFirewallRm builds the command that takes hosts the user added off the allowlist, once the user agrees.
func newFirewallRm() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm <host>...",
		Short: "Remove hosts",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := homeConfig()
			// Hand back a home that fails to load
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			dry := c
			dry.Hosts = slices.Clone(c.Hosts)
			// Name what removing each host does, before anything changes
			for _, name := range args {
				h, _ := dry.Find(name)
				kept, err := dry.Remove(name)
				// Hand back a removal the allowlist turns away
				if err != nil {
					return err
				}
				// Name a host that stays for what still needs it
				if len(kept) > 0 {
					fmt.Fprintf(out, "  ~ %s stays open, still needed by %s\n", name, words.JoinAnd(kept))
					continue
				}
				fmt.Fprintf(out, "  - %s:%s\n", name, config.JoinPorts(h.Ports))
			}
			ok, err := confirm(cmd.InOrStdin(), out, isTerminal(cmd.InOrStdin()), yes)
			// Hand back an answer that fails to arrive
			if err != nil {
				return err
			}
			// Leave the allowlist as it was when the user declines
			if !ok {
				fmt.Fprintln(out, "Left the allowlist as it was")
				return nil
			}
			return change(cmd, func(c *config.Config) error {
				// Remove every host the command names
				for _, name := range args {
					kept, err := c.Remove(name)
					// Hand back a removal the allowlist turns away
					if err != nil {
						return err
					}
					// Name what still needs a host that stays
					if len(kept) > 0 {
						fmt.Fprintf(out, "Kept %s, still needed by %s\n", name, words.JoinAnd(kept))
						continue
					}
					fmt.Fprintf(out, "Removed %s\n", name)
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove without asking")
	return cmd
}

// showFirewall writes the allowed hosts with their traffic and the names the gateway refused, as tables or as JSON.
func showFirewall(cmd *cobra.Command, limit int, asJSON bool) error {
	c, err := homeConfig()
	// Hand back a home that fails to load
	if err != nil {
		return err
	}
	_, store, err := loadSecrets()
	// Hand back a secrets file that fails to load
	if err != nil {
		return err
	}
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return err
	}
	record, running := askGateway(cmd, dir)
	blocked := capped(record.Refused(), limit)
	// Hand both lists to a script that asked for JSON
	if asJSON {
		return writeFirewallJSON(cmd, c, record, blocked)
	}
	out := cmd.OutOrStdout()
	now := time.Now()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(menu.HostHeader, "\t"))
	waiting, dead, expired := false, false, false
	// Write a row per host, in the order the file holds them
	for _, h := range c.Hosts {
		_, set := store.Value(h.Secret)
		waits := h.Secret != "" && !set
		var t menu.Traffic
		// Show what the host moved where the gateway recorded it
		if recorded, ok := record.Hosts[h.Name]; ok {
			t = hostTraffic(recorded, now)
			dead = dead || recorded.Dead(now)
		}
		waiting, expired = waiting || waits, expired || h.Expired(now)
		fmt.Fprintln(w, strings.Join(append([]string{h.Name}, menu.HostRow(h, t, waits, now)...), "\t"))
	}
	// Hand back a table that fails to write
	if err := w.Flush(); err != nil {
		return err
	}
	// Leave a line between the table and the notes below it
	if waiting || dead || expired {
		fmt.Fprintln(out)
	}
	// Explain the mark below a table that carries it
	if waiting {
		fmt.Fprintln(out, "! waits for its value: echo \"$TOKEN\" | o3s secret add <name>")
	}
	// Point a dead host at the command that removes it
	if dead {
		fmt.Fprintln(out, "dead: no data in 30 days, remove it with o3s firewall rm <host>")
	}
	// Point the expired hosts at the command that removes them
	if expired {
		fmt.Fprintln(out, "expired: past its end time, remove these with o3s firewall prune")
	}
	fmt.Fprintln(out)
	// Name where the blocked names come from while the stack is down
	if !running {
		fmt.Fprintln(out, "Blocked names show while the stack runs")
		return nil
	}
	// Say that nothing is blocked yet
	if len(blocked) == 0 {
		fmt.Fprintln(out, "Nothing blocked yet, names the cage asks for and the firewall refuses show here")
		return nil
	}
	w = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(menu.BlockedHeader, "\t"))
	// Write a row per refused name
	for _, b := range blocked {
		fmt.Fprintln(w, strings.Join(append([]string{b.Name}, menu.BlockedRow(b.Attempts, traffic.Ago(b.LastSeen, now))...), "\t"))
	}
	return w.Flush()
}

// newFirewallPrune builds the command that takes every expired host off the allowlist, once the user agrees.
func newFirewallPrune() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove every host whose end time has come",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := workingHome()
			// Hand back a working folder that stays unknown
			if err != nil {
				return err
			}
			c, err := config.Load(dir)
			// Hand back a home that fails to load
			if err != nil {
				return err
			}
			now := time.Now()
			out := cmd.OutOrStdout()
			var names []string
			// Name every expired host
			for _, h := range c.Expired(now) {
				names = append(names, h.Name)
			}
			// Say that nothing has expired
			if len(names) == 0 {
				fmt.Fprintln(out, "No expired host")
				return nil
			}
			fmt.Fprintf(out, "Removes the expired hosts %s from %s.\n", strings.Join(names, ", "), config.File)
			ok, err := confirm(cmd.InOrStdin(), out, isTerminal(cmd.InOrStdin()), yes)
			// Hand back an answer that fails to arrive
			if err != nil {
				return err
			}
			// Leave the allowlist as it was when the user declines
			if !ok {
				fmt.Fprintln(out, "Left the allowlist as it was")
				return nil
			}
			return change(cmd, func(c *config.Config) error {
				// Name every host that leaves
				for _, name := range c.Prune(now) {
					fmt.Fprintf(out, "Removed %s\n", name)
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove without asking")
	return cmd
}

// hostTraffic is how what the gateway recorded for a host reads: the data it moved, and when it last moved any, marked
// dead after a whole window without data.
func hostTraffic(t traffic.Host, now time.Time) menu.Traffic {
	seen := traffic.Ago(t.LastSeen, now)
	// Mark a host that moved no data for a whole window
	if t.Dead(now) {
		seen += ", dead"
	}
	return menu.Traffic{Data: traffic.Size(t.Bytes), Seen: seen}
}

// capped keeps the first entries up to a limit, and every entry for a limit of 0.
func capped[T any](entries []T, limit int) []T {
	// Keep every entry within the limit
	if limit <= 0 || len(entries) <= limit {
		return entries
	}
	return entries[:limit]
}

// writeFirewallJSON writes the allowed hosts with their traffic and the refused names as one JSON document.
func writeFirewallJSON(cmd *cobra.Command, c config.Config, record traffic.Record, blocked []traffic.Refused) error {
	type allowedRow struct {
		Host     string   `json:"host"`
		Ports    []int    `json:"ports"`
		Bytes    int64    `json:"bytes"`
		LastSeen int64    `json:"last_seen"`
		Expires  string   `json:"expires,omitempty"`
		Secret   string   `json:"secret,omitempty"`
		By       []string `json:"by"`
	}
	type blockedRow struct {
		Host     string `json:"host"`
		Attempts int64  `json:"attempts"`
		LastSeen int64  `json:"last_seen"`
	}
	doc := struct {
		Allowed []allowedRow `json:"allowed"`
		Blocked []blockedRow `json:"blocked"`
	}{Allowed: []allowedRow{}, Blocked: []blockedRow{}}
	// Take every allowed host with what the gateway recorded for it
	for _, h := range c.Hosts {
		t := record.Hosts[h.Name]
		doc.Allowed = append(doc.Allowed, allowedRow{Host: h.Name, Ports: h.Ports, Bytes: t.Bytes, LastSeen: t.LastSeen, Expires: h.Expires, Secret: h.Secret, By: h.By})
	}
	// Take every refused name
	for _, b := range blocked {
		doc.Blocked = append(doc.Blocked, blockedRow{Host: b.Name, Attempts: b.Attempts, LastSeen: b.LastSeen})
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// askGateway asks the home's running gateway for its record and reports whether it runs, warning about and leaving out a record that fails to read.
func askGateway(cmd *cobra.Command, dir string) (traffic.Record, bool) {
	record, running, err := traffic.Read(cmd.Context(), dir)
	// Leave the traffic out where its record fails to read
	if err != nil {
		warn(cmd.ErrOrStderr(), "%v", err)
		return traffic.Record{}, running
	}
	return record, running
}

// change applies a change to the home, saves it, and restarts the stack so it takes effect.
func change(cmd *cobra.Command, apply func(*config.Config) error) error {
	dir, err := edit(apply)
	// Hand back a change that fails to apply or save
	if err != nil {
		return err
	}
	return restartStack(cmd, dir)
}

// restartStack restarts the home's stack so a saved change takes effect, and names how it does.
func restartStack(cmd *cobra.Command, dir string) error {
	running, err := restart.Stack(cmd.Context(), dir)
	// Hand back a stack that fails to restart, its settings already saved
	if err != nil {
		return fmt.Errorf("saved %s, restart the stack to apply it: %w", config.File, err)
	}
	// Name how the change takes effect
	if running {
		fmt.Fprintln(cmd.OutOrStdout(), "Restarted gateway")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "The stack picks the change up when it starts")
	return nil
}

// edit applies a change to the home and saves it, and hands back the home's folder.
func edit(apply func(*config.Config) error) (string, error) {
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return "", err
	}
	c, err := config.Load(dir)
	// Hand back a home that fails to load
	if err != nil {
		return "", err
	}
	// Leave the file as it was when the change fails
	if err := apply(&c); err != nil {
		return "", err
	}
	return dir, c.Save(dir)
}

// allowed confirms a host with its ports, and its secret where it carries one.
func allowed(h config.Host, now time.Time) string {
	line := fmt.Sprintf("Allowed %s:%s", h.Name, config.JoinPorts(h.Ports))
	// Add the secret and where it goes
	if h.Secret != "" {
		line += fmt.Sprintf(" with %s in %s", h.Secret, strings.Join(h.Inject, ", "))
	}
	// Add when a host with an end closes
	if h.Expires != "" {
		line += " until " + config.ClosesAt(h.Expires, now)
	}
	return line
}
