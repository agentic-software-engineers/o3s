package vm

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// State is where a home's VM stands, as every command that shows or takes the VM reads it.
type State string

// The states a home's VM is in.
const (
	// Absent is a home whose VM the manager lacks, gone or never made.
	Absent State = "absent"
	// Unfinished is a VM that o3s init started making and has yet to finish.
	Unfinished State = "unfinished"
	// Stopped is a finished VM that is not running.
	Stopped State = "stopped"
	// Running is a finished VM that runs.
	Running State = "running"
)

// ReadyFile marks a host folder whose home o3s init finished making in its VM.
var ReadyFile = filepath.Join(".o3s", "ready")

// MarkReady records that the home in a folder's VM is made, so the VM reads as finished from then on.
func MarkReady(dir string) error {
	return os.WriteFile(filepath.Join(dir, ReadyFile), nil, 0o600)
}

// ready reports whether the home in a folder's VM is made.
func ready(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ReadyFile))
	return !errors.Is(err, fs.ErrNotExist)
}

// State reads where the VM of the home in a folder stands, with the VM's name.
func (l Lima) State(ctx context.Context, dir string) (State, string, error) {
	name, err := Find(dir)
	// Hand back a folder without a VM
	if err != nil {
		return Absent, "", err
	}
	status, err := l.Status(ctx, name)
	// Hand back a VM whose state stays unknown
	if err != nil {
		return "", name, err
	}
	// Read the state the manager names, a VM o3s init has yet to finish read as unfinished whatever it does
	switch {
	case status == "Absent":
		return Absent, name, nil
	case !ready(dir):
		return Unfinished, name, nil
	case strings.EqualFold(status, "Running"):
		return Running, name, nil
	}
	return Stopped, name, nil
}
