package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hansehart/o3s/internal/paths"
	"github.com/hansehart/o3s/internal/vm"
)

// errExit carries the exit code of a command that ran in the VM and failed, so o3s fails with it too.
type errExit struct{ code int }

// Error names the exit code the command in the VM failed with.
func (e errExit) Error() string {
	return fmt.Sprintf("the command in the VM exited with %d", e.code)
}

// machine is the VM of the home in a folder: the manager that runs it, writing its own records to the home's VM log,
// the folder, and the VM's name.
type machine struct {
	lima      vm.Lima
	dir, name string
}

// homeMachine finds the VM of the working folder's home, and reports whether the home lives in one.
func homeMachine() (machine, bool, error) {
	dir, err := workingHome()
	// Hand back a working folder that stays unknown
	if err != nil {
		return machine{}, false, err
	}
	name, err := vm.Find(dir)
	// Report a home that lives on this machine
	if errors.Is(err, vm.ErrNoVM) {
		return machine{}, false, nil
	}
	// Hand back an ID that fails to read
	if err != nil {
		return machine{}, true, err
	}
	m, err := managed(dir, name)
	return m, true, err
}

// managed builds the machine of a VM by its name, its manager writing to the home's VM log.
func managed(dir, name string) (machine, error) {
	lima, err := vm.Manager()
	// Hand back a host without the VM manager
	if err != nil {
		return machine{}, err
	}
	return machine{lima: lima.LoggingTo(vmLog{path: vmLogPath(dir)}), dir: dir, name: name}, nil
}

// state reads where the machine's VM stands.
func (m machine) state(ctx context.Context) (vm.State, error) {
	s, _, err := m.lima.State(ctx, m.dir)
	return s, err
}

// vmLogPath is where the VM manager's records for a home go, in the folder o3s keeps for the home.
func vmLogPath(dir string) string {
	return filepath.Join(paths.Home(dir), "vm.log")
}

// vmLog appends the VM manager's records to the home's VM log, opening it for each write so no file stays open.
type vmLog struct {
	path string
}

// Write appends records to the log, and drops them where the log fails to open.
func (l vmLog) Write(p []byte) (int, error) {
	// Drop the records where the log's folder fails to appear
	if err := os.MkdirAll(filepath.Dir(l.path), 0o750); err != nil {
		return len(p), nil
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	// Drop the records where the log fails to open
	if err != nil {
		return len(p), nil
	}
	defer f.Close()
	return f.Write(p)
}
