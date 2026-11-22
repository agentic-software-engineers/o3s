package cli

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/hansehart/o3s/internal/vm"
)

// show writes where the machine's VM stands.
func (m machine) show(cmd *cobra.Command) error {
	state, err := m.state(cmd.Context())
	// Hand back a VM whose state stays unknown
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "PART  STATE\nvm    %s\n", state)
	return nil
}

// start starts a finished VM, and points one o3s init has yet to finish at init.
func (m machine) start(cmd *cobra.Command) error {
	state, err := m.state(cmd.Context())
	// Hand back a VM whose state stays unknown
	if err != nil {
		return err
	}
	// Point an unfinished or absent VM at init, which makes and finishes it
	if state == vm.Unfinished || state == vm.Absent {
		return fmt.Errorf("the VM %s is %s, run o3s init to make it", m.name, state)
	}
	// Hand back a VM that fails to start, in the manager's words
	if err := m.lima.Start(cmd.Context(), m.name, cmd.ErrOrStderr()); err != nil {
		return vmFailure("start the VM "+m.name, err, vmLogPath(m.dir))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "The VM %s runs\n", m.name)
	return nil
}

// stop stops the machine's VM.
func (m machine) stop(cmd *cobra.Command) error {
	// Hand back a VM that keeps running, in the manager's words
	if err := m.lima.Stop(cmd.Context(), m.name, cmd.ErrOrStderr()); err != nil {
		return vmFailure("stop the VM "+m.name, err, vmLogPath(m.dir))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Stopped the VM %s\n", m.name)
	return nil
}

// remove deletes the machine's VM and its disk.
func (m machine) remove(cmd *cobra.Command) error {
	// Hand back a VM that stays, in the manager's words
	if err := m.lima.Delete(cmd.Context(), m.name, cmd.ErrOrStderr()); err != nil {
		return vmFailure("delete the VM "+m.name, err, vmLogPath(m.dir))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted the VM %s\n", m.name)
	return nil
}

// shell opens a shell in the machine's running VM, or runs a command there, failing with the command's own exit code,
// and points a VM that stays down at the command that starts it.
func (m machine) shell(cmd *cobra.Command, args []string) error {
	state, err := m.state(cmd.Context())
	// Hand back a VM whose state stays unknown
	if err != nil {
		return err
	}
	// Point a stopped VM at what starts it
	if state != vm.Running {
		return notRunning(m.name, state)
	}
	err = m.lima.Exec(cmd.Context(), m.name, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args...)
	// Fail with the exit code of a command that ran and failed
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return errExit{code: exit.ExitCode()}
	}
	return err
}

// notRunning names a VM that stays down, and what starts it: init for one it has yet to finish, cage start
// otherwise.
func notRunning(name string, state vm.State) error {
	// Point a VM init has yet to finish at init
	if state == vm.Unfinished || state == vm.Absent {
		return fmt.Errorf("the VM %s is %s, run o3s init to make it", name, state)
	}
	return fmt.Errorf("the VM %s is %s, start it with o3s cage start", name, state)
}
