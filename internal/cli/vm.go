package cli

import (
	"fmt"

	"github.com/hansehart/o3s/internal/vm"
)

// errExit carries the exit code of a command that ran in the VM and failed, so o3s fails with it too.
type errExit struct{ code int }

// Error names the exit code the command in the VM failed with.
func (e errExit) Error() string {
	return fmt.Sprintf("the command in the VM exited with %d", e.code)
}

// homeVM finds the VM manager and the name of the working folder's VM.
func homeVM() (vm.Lima, string, error) {
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return vm.Lima{}, "", err
	}
	name, err := vm.Find(dir)
	// Point a folder without a VM at the command that makes one
	if err != nil {
		return vm.Lima{}, "", err
	}
	lima, err := vm.Manager()
	return lima, name, err
}
