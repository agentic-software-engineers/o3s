// Package cli turns a command line into changes to an o3s home.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/registry"
	"github.com/hansehart/o3s/internal/secrets"
	"github.com/hansehart/o3s/internal/tui"
	"github.com/hansehart/o3s/internal/vm"
)

// ErrUsage marks a command line with missing or malformed arguments.
var ErrUsage = errors.New("usage error")

// Streams are where a command reads its input and writes its output.
type Streams struct {
	In       io.Reader
	Out, Err io.Writer
}

// Run executes a command line and returns 0, 2 for a usage error, the exit code of a command run in the VM, or 1 for any other failure.
func Run(ctx context.Context, args []string, s Streams) int {
	home.Version = version()
	home.Digests = Digests
	home.Executable, _ = executable()
	// Lead a user at a terminal through the menu when the command is left out
	if len(args) == 0 && isInteractive(s.In, s.Out) {
		return runMenu(ctx, s)
	}
	root := newRoot(s)
	root.SetArgs(append([]string{}, args...))
	cmd, err := root.ExecuteContextC(ctx)
	// Report success
	if err == nil {
		return 0
	}
	// Pass on the exit code of a command that ran in the VM and failed, which printed its own failure
	if exit, ok := errors.AsType[errExit](err); ok {
		return exit.code
	}
	err = inItsVM(ctx, err)
	fmt.Fprintf(s.Err, "error: %v\n", err)
	// Leave the error where the menu asks for it, which outlasts the terminal it printed on
	if file := os.Getenv(tui.ErrorFileEnv); file != "" {
		_ = os.WriteFile(file, fmt.Appendf(nil, "error: %v\n", err), 0o600) //nolint:gosec // the menu names a file it made for this run alone
	}
	// Point a usage error at the help of the command it hit
	if isUsage(err) {
		fmt.Fprintf(s.Err, "Run '%s --help' for usage.\n", cmd.CommandPath())
		return 2
	}
	return 1
}

// inItsVM names the way into the home's VM where a command meets a folder whose home lives in that VM: in through the
// VM's shell where it runs, and what starts or finishes it otherwise.
func inItsVM(ctx context.Context, err error) error {
	// Keep the error of a folder that lacks a home of any kind
	if !errors.Is(err, config.ErrNoHome) {
		return err
	}
	m, inVM, merr := homeMachine()
	// Keep the error of a folder whose home lacks a VM
	if !inVM {
		return err
	}
	// Point at the VM's shell where the VM's state stays unknown
	if merr != nil {
		return errors.New("this home lives in its VM, run its commands there: o3s cage shell --vm")
	}
	state, serr := m.state(ctx)
	// Point a stopped VM at what starts or finishes it
	if serr == nil && state != vm.Running {
		return fmt.Errorf("this home lives in its VM: %w", notRunning(m.name, state))
	}
	return errors.New("this home lives in its VM, run its commands there: o3s cage shell --vm")
}

// isUsage reports whether an error comes from the command line.
func isUsage(err error) bool {
	// Read a broken settings file as a failure of the home, whatever rule it breaks
	if errors.Is(err, config.ErrMalformed) {
		return false
	}
	return errors.Is(err, ErrUsage) || errors.Is(err, config.ErrInvalid) || errors.Is(err, secrets.ErrInvalid) || errors.Is(err, registry.ErrInvalid)
}

// warn names something the user should know on stderr, so it stays apart from a command's output.
func warn(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "warning: "+format+"\n", args...)
}
