package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/hostd"
)

// Limits the CLI keeps a host process running with.
const (
	// hostLogLimit is the size at which the process's log moves aside.
	hostLogLimit = 10 << 20
	// hostReady is how long a new process may take to answer.
	hostReady = 10 * time.Second
	// hostGone is how long a process asked to stop may take to go.
	hostGone = 5 * time.Second
	// hostAttempts is how often the CLI starts a process before it gives up.
	hostAttempts = 3
)

// newHost builds the commands that run the home's host process, and start, stop and show it.
func newHost() *cobra.Command {
	c := &cobra.Command{
		Use:    "host",
		Short:  "Run the home's host process, which serves its cage's connections",
		Hidden: true,
		Args:   usage(cobra.NoArgs),
	}
	c.AddCommand(newHostServe(), newHostStart(), newHostStop(), newHostStatus())
	return c
}

// newHostServe builds the command that is the host process itself.
func newHostServe() *cobra.Command {
	var home string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve a home's cage until a signal or a shutdown arrives",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := hostd.PlaceOf(home)
			// Hand back a place that fails to settle
			if err != nil {
				return err
			}
			w, err := hostd.Rotating(p.Log(), hostLogLimit)
			// Hand back a log that fails to open
			if err != nil {
				return err
			}
			defer w.Close()
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return hostd.Serve(ctx, p, hostd.ServeOptions{Version: version(), Log: slog.New(slog.NewJSONHandler(w, nil))})
		},
	}
	cmd.Flags().StringVar(&home, "home", "", "the home's folder")
	_ = cmd.MarkFlagRequired("home")
	return cmd
}

// newHostStart builds the command that keeps the home's host process running at this version.
func newHostStart() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the home's host process, or keep the one that runs",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := hostPlace()
			// Hand back a home whose place fails to settle
			if err != nil {
				return err
			}
			exe, err := executable()
			// Hand back a binary whose path stays unknown
			if err != nil {
				return err
			}
			info, err := hostd.Ensure(cmd.Context(), p, hostd.EnsureOptions{
				Version:  version(),
				Spawn:    func() error { return hostd.Spawn(cmd.Context(), exe, p) },
				Ready:    hostReady,
				Gone:     hostGone,
				Attempts: hostAttempts,
			})
			// Hand back a process that stays silent
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "host process running, pid %d\n", info.PID)
			return nil
		},
	}
}

// newHostStop builds the command that stops the home's host process.
func newHostStop() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the home's host process",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := hostPlace()
			// Hand back a home whose place fails to settle
			if err != nil {
				return err
			}
			// Hand back a process that fails to take the stop
			if err := hostd.Shutdown(cmd.Context(), p.Control()); err != nil {
				return fmt.Errorf("host process out of reach: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "host process stopping")
			return nil
		},
	}
}

// newHostStatus builds the command that shows whether the home's host process runs.
func newHostStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the home's host process runs, with its pid and version",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := hostPlace()
			// Hand back a home whose place fails to settle
			if err != nil {
				return err
			}
			info, err := hostd.GetInfo(cmd.Context(), p.Control())
			// Report a process that stays silent
			if err != nil {
				return fmt.Errorf("host process out of reach: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "host process running, pid %d, version %s\n", info.PID, info.Version)
			return nil
		},
	}
}

// hostPlace settles the place of the host process of the home the command runs in.
func hostPlace() (hostd.Place, error) {
	dir, err := cageHome()
	// Hand back a folder that lacks a home
	if err != nil {
		return hostd.Place{}, err
	}
	return hostd.PlaceOf(dir)
}
