package cli

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/vm"
)

// errExit carries the exit code of a command that ran in the VM and failed, so o3s fails with it too.
type errExit struct{ code int }

func (e errExit) Error() string { return fmt.Sprintf("the command in the VM exited with %d", e.code) }

// newVM builds the commands that reach the home's VM.
func newVM() *cobra.Command {
	c := group("vm", "Reach the VM the home runs in")
	c.AddCommand(newVMSSH(), newVMRm())
	return c
}

// newVMSSH builds the command that runs a command in the home's VM.
func newVMSSH() *cobra.Command {
	return &cobra.Command{
		Use:   "ssh -- <command> [args...]",
		Short: "Run a command in the home's VM",
		Args:  usage(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			lima, name, err := homeVM()
			// Hand back a host that can't reach the VM
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

// newVMRm builds the command that deletes the home's VM.
func newVMRm() *cobra.Command {
	return &cobra.Command{
		Use:   "rm",
		Short: "Delete the home's VM and its disk",
		Args:  usage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			lima, name, err := homeVM()
			// Hand back a host that can't reach the VM
			if err != nil {
				return err
			}
			// Hand back a VM that stays
			if err := lima.Delete(cmd.Context(), name, cmd.ErrOrStderr()); err != nil {
				return fmt.Errorf("delete the VM %s: %w", name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted the VM %s\n", name)
			return nil
		},
	}
}

// bootVM creates the home's VM where it is missing and starts it.
func bootVM(cmd *cobra.Command, dir string) error {
	lima, err := vm.Find()
	// Hand back a host without Lima or with an engine it can't run
	if err != nil {
		return err
	}
	name := vm.Name(dir)
	// Hand back a VM that fails to come up
	if err := lima.Up(cmd.Context(), name, cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("boot the VM %s: %w", name, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "The VM %s runs\n", name)
	return nil
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
