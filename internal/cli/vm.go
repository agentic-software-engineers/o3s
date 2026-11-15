package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/vm"
)

// errExit carries the exit code of a command that ran in the VM and failed, so o3s fails with it too.
type errExit struct{ code int }

func (e errExit) Error() string { return fmt.Sprintf("the command in the VM exited with %d", e.code) }

// newVM builds the commands that run the home's VM, named the way Lima and Podman machine name theirs.
func newVM() *cobra.Command {
	c := group("vm", "Run the VM the home lives in")
	c.AddCommand(
		newVMLifecycle("start", "Create the home's VM where it is missing and start it", "The VM %s runs\n", vm.Lima.Start),
		newVMShell(),
		newVMLifecycle("stop", "Stop the home's VM", "Stopped the VM %s\n", vm.Lima.Stop),
		newVMLifecycle("rm", "Delete the home's VM and its disk", "Deleted the VM %s\n", vm.Lima.Delete),
	)
	return c
}

// newVMLifecycle builds a command that takes the home's VM one step through its life, with Lima's progress on stderr.
func newVMLifecycle(use, short, done string, step func(vm.Lima, context.Context, string, io.Writer) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			lima, name, err := homeVM()
			// Hand back why the VM stays out of reach
			if err != nil {
				return err
			}
			// Hand back a VM that fails to take the step
			if err := step(lima, cmd.Context(), name, cmd.ErrOrStderr()); err != nil {
				return fmt.Errorf("%s the VM %s: %w", use, name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), done, name)
			return nil
		},
	}
}

// newVMShell builds the command that runs a command in the home's VM, or opens its shell for a bare call.
func newVMShell() *cobra.Command {
	return &cobra.Command{
		Use:   "shell [command] [args...]",
		Short: "Open a shell in the home's VM, or run a command there",
		Args:  usage(cobra.ArbitraryArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			lima, name, err := homeVM()
			// Hand back why the VM stays out of reach
			if err != nil {
				return err
			}
			err = lima.Exec(cmd.Context(), name, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args...)
			// Fail with the exit code of a command that ran and failed
			if exit, ok := errors.AsType[*exec.ExitError](err); ok {
				return errExit{code: exit.ExitCode()}
			}
			return err
		},
	}
}

// homeVM finds Lima and the name of the working folder's VM.
func homeVM() (vm.Lima, string, error) {
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return vm.Lima{}, "", err
	}
	lima, err := vm.Find()
	return lima, vm.Name(dir), err
}
