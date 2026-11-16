package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/restart"
	"github.com/hansehart/o3s/internal/traffic"
)

// newFirewall builds the commands that allow, remove and list hosts.
func newFirewall() *cobra.Command {
	fw := group("firewall", "Allow or remove the hosts the cage may reach")
	fw.AddCommand(newFirewallAdd(), newFirewallRm(), newFirewallLs(), newFirewallLog(), newFirewallPrune())
	return fw
}

// newFirewallAdd builds the command that allows a host, or updates the one it names.
func newFirewallAdd() *cobra.Command {
	var ch config.Change
	var forValue string
	var yes bool
	cmd := &cobra.Command{
		Use:   "add <host>",
		Short: "Allow a host, or update it",
		Args:  usage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ch.Name = args[0]
			now := time.Now()
			// Give the host the end time its duration reaches
			if forValue != "" {
				d, err := parseFor(forValue)
				// Hand back a duration that reads as no time
				if err != nil {
					return err
				}
				ch.Expires = config.EndTime(now.Add(d))
			}
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
			// Ask before a host closes sooner than it would, naming everyone it reaches
			if h, ok := c.Find(ch.Name); ok && narrows(h.Expires, ch.Expires) {
				out := cmd.OutOrStdout()
				consequences(out, h, forValue, ch.Expires, now)
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
				diffs, err := c.Allow(ch)
				// Hand back a change the allowlist turns away
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				// List the fields an update changed, under the host they belong to
				if len(diffs) > 0 {
					fmt.Fprintln(out, ch.Name)
					// Show every changed field with its value before and after
					for _, d := range diffs {
						// Show end times as the local clock time they close at
						if d.Field == "expires" {
							d.Old, d.New = closesAtOrNone(d.Old, now), closesAt(d.New, now)
						}
						fmt.Fprintf(out, "  ~ %s %s -> %s\n", d.Field, orNone(d.Old), orNone(d.New))
					}
				}
				h, _ := c.Find(ch.Name)
				fmt.Fprintln(out, allowed(h, now))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&forValue, "for", "", "keep the host open this long, as seconds or a duration like 20m or 1h (default for good)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "close a host sooner without asking")
	cmd.Flags().IntSliceVarP(&ch.Ports, "port", "p", nil, "a port to allow, 1-65535, repeatable (default 443)")
	cmd.Flags().StringVar(&ch.Secret, "secret", "", "the name of the secret the proxy injects for this host")
	cmd.Flags().StringSliceVar(&ch.Inject, "inject", nil, "where the secret goes: header, query or body, repeatable (default header)")
	return cmd
}

// newFirewallRm builds the command that takes a host the user added off the allowlist.
func newFirewallRm() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <host>",
		Short: "Remove a host",
		Args:  usage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return change(cmd, func(c *config.Config) error {
				kept, err := c.Remove(name)
				// Hand back a removal the allowlist turns away
				if err != nil {
					return err
				}
				// Name what still needs a host that stays
				if len(kept) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Kept %s, still needed by %s\n", name, strings.Join(kept, ", "))
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", name)
				return nil
			})
		},
	}
}

// newFirewallLs builds the command that lists the allowlist.
func newFirewallLs() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List the hosts",
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
			_, store, err := loadSecrets()
			// Hand back a secrets file that fails to load
			if err != nil {
				return err
			}
			record, _ := askGateway(cmd, dir)
			now := time.Now()
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "HOST\tPORTS\tDATA\tLAST SEEN\tEXPIRES\tSECRET\tBY")
			waiting, dead, expired := false, false, false
			// Write a row per host, in the order the file holds them
			for _, h := range c.Hosts {
				secret := h.Secret
				// Flag a secret that waits for its value
				if _, ok := store.Value(secret); secret != "" && !ok {
					secret += " !"
					waiting = true
				}
				data, seen := "–", "–"
				// Show what the host moved where the gateway recorded it
				if t, ok := record.Hosts[h.Name]; ok {
					data, seen = traffic.Size(t.Bytes), traffic.Ago(t.LastSeen, now)
					// Mark a host that moved no data for a whole window
					if t.Dead(now) {
						seen += ", dead"
						dead = true
					}
				}
				expires := until(h.Expires, now)
				expired = expired || h.Expired(now)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", h.Name, joinPorts(h.Ports), data, seen, expires, secret, strings.Join(h.By, ", "))
			}
			// Hand back a table that fails to write
			if err := w.Flush(); err != nil {
				return err
			}
			// Leave a line between the table and the notes below it
			if waiting || dead || expired {
				fmt.Fprintln(cmd.OutOrStdout())
			}
			// Explain the mark below a table that carries it
			if waiting {
				fmt.Fprintln(cmd.OutOrStdout(), "! waits for its value: echo \"$TOKEN\" | o3s secret add <NAME>")
			}
			// Point a dead host at the command that removes it
			if dead {
				fmt.Fprintln(cmd.OutOrStdout(), "dead: no data in 30 days, remove it with o3s firewall rm <host>")
			}
			// Point the expired hosts at the command that removes them
			if expired {
				fmt.Fprintln(cmd.OutOrStdout(), "expired: past its end time, remove these with o3s firewall prune")
			}
			return nil
		},
	}
}

// newFirewallLog builds the command that lists the names the gateway refused and the data each allowed host moved.
func newFirewallLog() *cobra.Command {
	var limit int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "log",
		Short: "List the blocked names and the data each allowed host moved",
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
			record, running := askGateway(cmd, dir)
			blocked, allowed := capped(record.Refused(), limit), capped(active(c, record), limit)
			// Hand both tables to a script that asked for JSON
			if asJSON {
				return writeLogJSON(cmd, blocked, allowed)
			}
			out := cmd.OutOrStdout()
			// Point at starting a stack that is down, since its gateway holds the record
			if !running {
				fmt.Fprintln(out, "Start the stack to see its traffic")
				return nil
			}
			// Say that nothing is recorded yet
			if len(blocked) == 0 && len(allowed) == 0 {
				fmt.Fprintln(out, "No traffic recorded yet, the gateway records it every minute")
				return nil
			}
			now := time.Now()
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "BLOCKED\tATTEMPTS\tLAST SEEN")
			// Write a row per refused name
			for _, b := range blocked {
				fmt.Fprintf(w, "%s\t%d\t%s\n", b.Name, b.Attempts, traffic.Ago(b.LastSeen, now))
			}
			// Hand back a table that fails to write
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Fprintln(out)
			w = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ALLOWED\tDATA\tLAST SEEN")
			// Write a row per allowed host
			for _, a := range allowed {
				seen := traffic.Ago(a.LastSeen, now)
				// Mark a host that moved no data for a whole window, as the allowlist does
				if a.Dead(now) {
					seen += ", dead"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, traffic.Size(a.Bytes), seen)
			}
			// Hand back a table that fails to write
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Fprintln(out, "\nAllow a blocked host with: o3s firewall add <host> --port 443")
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "show up to this many entries per table, 0 for all")
	cmd.Flags().BoolVar(&asJSON, "json", false, "write both tables as JSON")
	return cmd
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

// recorded is an allowed host with the traffic the gateway recorded for it.
type recorded struct {
	Name string
	traffic.Host
}

// active lists the allowed hosts the gateway recorded, the most recently active first.
func active(c config.Config, record traffic.Record) []recorded {
	var out []recorded
	// Take every allowed host the record holds
	for _, h := range c.Hosts {
		// Take a host the gateway counted
		if t, ok := record.Hosts[h.Name]; ok {
			out = append(out, recorded{Name: h.Name, Host: t})
		}
	}
	slices.SortStableFunc(out, func(a, b recorded) int { return cmp.Compare(b.LastSeen, a.LastSeen) })
	return out
}

// capped keeps the first entries up to a limit, and every entry for a limit of 0.
func capped[T any](entries []T, limit int) []T {
	// Keep every entry within the limit
	if limit <= 0 || len(entries) <= limit {
		return entries
	}
	return entries[:limit]
}

// writeLogJSON writes both tables as one JSON document.
func writeLogJSON(cmd *cobra.Command, blocked []traffic.Refused, allowed []recorded) error {
	type blockedRow struct {
		Host     string `json:"host"`
		Attempts int64  `json:"attempts"`
		LastSeen int64  `json:"last_seen"`
	}
	type allowedRow struct {
		Host     string `json:"host"`
		Bytes    int64  `json:"bytes"`
		LastSeen int64  `json:"last_seen"`
	}
	doc := struct {
		Blocked []blockedRow `json:"blocked"`
		Allowed []allowedRow `json:"allowed"`
	}{Blocked: []blockedRow{}, Allowed: []allowedRow{}}
	// Take every refused name
	for _, b := range blocked {
		doc.Blocked = append(doc.Blocked, blockedRow{Host: b.Name, Attempts: b.Attempts, LastSeen: b.LastSeen})
	}
	// Take every allowed host
	for _, a := range allowed {
		doc.Allowed = append(doc.Allowed, allowedRow{Host: a.Name, Bytes: a.Bytes, LastSeen: a.LastSeen})
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
	line := fmt.Sprintf("Allowed %s:%s", h.Name, joinPorts(h.Ports))
	// Add the secret and where it goes
	if h.Secret != "" {
		line += fmt.Sprintf(" with %s in %s", h.Secret, strings.Join(h.Inject, ", "))
	}
	// Add when a host with an end closes
	if h.Expires != "" {
		line += " until " + closesAt(h.Expires, now)
	}
	return line
}

// joinPorts writes ports as one comma-separated value.
func joinPorts(ports []int) string {
	parts := make([]string, len(ports))
	// Write every port
	for i, p := range ports {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ",")
}

// orNone shows an empty value as none.
func orNone(v string) string {
	// Name the absence of a value
	if v == "" {
		return "none"
	}
	return v
}
