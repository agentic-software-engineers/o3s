// Package vm runs the VM an o3s home lives in, with the engine each host system runs natively.
package vm

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/hansehart/o3s/internal/paths"
)

// template describes the machine every home's VM boots from.
//
//go:embed o3s.yaml
var template []byte

// Engines are the VM types o3s boots a home's VM with.
var Engines = []string{"qemu", "vz", "hcs"}

// ErrNoLima asks for the VM manager a home's VM runs with.
var ErrNoLima = errors.New("o3s runs a home's VM with Lima, put limactl on PATH (https://lima-vm.io/docs/installation/)")

// Lima runs the homes' VMs, kept apart from any VMs the user runs.
type Lima struct {
	path   string
	engine string
}

// Manager picks the engine for this host, or the one the user names, and the VM manager o3s installed or one on PATH.
func Manager() (Lima, error) {
	engine, err := Engine()
	// Hand back why the engine is turned away
	if err != nil {
		return Lima{}, err
	}
	name := "limactl"
	// Name the binary the way Windows finds a program
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	installed := filepath.Join(paths.Lima(), "bin", name)
	// Take the copy o3s installed
	if _, err := os.Stat(installed); err == nil {
		return Lima{path: installed, engine: engine}, nil
	}
	path, err := exec.LookPath("limactl")
	// Point at the manager's installation for a host that needs it
	if err != nil {
		return Lima{}, ErrNoLima
	}
	return Lima{path: path, engine: engine}, nil
}

// Engine is the VM type this host system runs natively, unless the user names another.
func Engine() (string, error) {
	// Take the engine the user names
	if engine := os.Getenv("O3S_ENGINE"); engine != "" {
		// Turn away an engine outside the ones o3s boots
		if !slices.Contains(Engines, engine) {
			return "", fmt.Errorf("O3S_ENGINE=%s, where o3s boots a VM with %s", engine, strings.Join(Engines, ", "))
		}
		return engine, nil
	}
	switch runtime.GOOS {
	case "darwin":
		return "vz", nil
	case "windows":
		return "hcs", nil
	default:
		return "qemu", nil
	}
}

// firstStart is how long a home's VM may take to boot the first time, its setup included, longer than the VM manager's default,
// which fits a VM that boots without one.
const firstStart = 30 * time.Minute

// Start boots a home's VM, created from the template on its first start, with its progress on out.
func (l Lima) Start(ctx context.Context, name string, out io.Writer) error {
	names, err := l.output(ctx, "list", "--quiet")
	// Hand back a manager that fails to list its VMs
	if err != nil {
		return err
	}
	// Start the VM the home already has
	if slices.Contains(strings.Fields(names), name) {
		return l.run(ctx, nil, out, out, "start", name)
	}
	args := []string{"start", "--name=" + name, "--vm-type=" + l.engine, "--timeout=" + firstStart.String()}
	// Name a DNS server for the engine that needs one set
	if l.engine == "hcs" {
		args = append(args, "--dns=8.8.8.8")
	}
	return l.run(ctx, bytes.NewReader(template), out, out, append(args, "-")...)
}

// Stop stops a home's VM.
func (l Lima) Stop(ctx context.Context, name string, out io.Writer) error {
	return l.run(ctx, nil, out, out, "stop", name)
}

// Exec runs a command in a home's VM, with the given streams, and hands back the command's failure.
func (l Lima) Exec(ctx context.Context, name string, in io.Reader, out, errOut io.Writer, args ...string) error {
	return l.run(ctx, in, out, errOut, append([]string{"shell", name}, args...)...)
}

// Delete removes a home's VM and its disk, stopping it first so its files are free to go.
func (l Lima) Delete(ctx context.Context, name string, out io.Writer) error {
	status, err := l.output(ctx, "list", "--format={{.Status}}", name)
	// Hand back a VM whose state stays unknown
	if err != nil {
		return err
	}
	// Stop the VM first where it runs
	if strings.TrimSpace(status) == "Running" {
		// Hand back a VM that keeps running
		if err := l.Stop(ctx, name, out); err != nil {
			return err
		}
	}
	return l.run(ctx, nil, out, out, "delete", "--force", name)
}

// run runs the VM manager for o3s's own VMs, in its automation mode.
func (l Lima) run(ctx context.Context, in io.Reader, out, errOut io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, l.path, append([]string{"--tty=false"}, args...)...) //nolint:gosec // o3s runs the limactl it found, with arguments it builds itself
	cmd.Env = append(os.Environ(), "LIMA_HOME="+paths.Lima())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errOut
	return cmd.Run()
}

// output runs the VM manager and hands back what it printed, with its error output in the failure.
func (l Lima) output(ctx context.Context, args ...string) (string, error) {
	var out, errOut bytes.Buffer
	// Hand back the error output along with the failure
	if err := l.run(ctx, nil, &out, &errOut, args...); err != nil {
		return "", fmt.Errorf("limactl %s: %w\n%s", strings.Join(args, " "), err, errOut.String())
	}
	return out.String(), nil
}
