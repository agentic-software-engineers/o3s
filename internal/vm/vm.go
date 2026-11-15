// Package vm runs the VM an o3s home lives in through Lima, one engine per host system, the way Podman machine drives one provider per system.
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

	"github.com/hansehart/o3s/internal/paths"
)

// template is the Lima template every home's VM boots from.
//
//go:embed o3s.yaml
var template []byte

// Engines are the Lima VM types o3s boots a home's VM with.
var Engines = []string{"qemu", "vz", "hcs"}

// ErrNoLima marks a host without limactl.
var ErrNoLima = errors.New("o3s runs a home's VM with Lima, put limactl on PATH (https://lima-vm.io/docs/installation/)")

// Lima drives limactl with o3s's own Lima home.
type Lima struct {
	path   string
	engine string
}

// Find picks the engine for this host, or the one O3S_ENGINE names, and the limactl o3s installed or one on PATH.
func Find() (Lima, error) {
	engine, err := Engine()
	// Hand back an engine the host can't run
	if err != nil {
		return Lima{}, err
	}
	name := "limactl"
	// Name the binary the way Windows finds a program
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	installed := filepath.Join(paths.Lima(), "bin", name)
	// Take the Lima o3s installed
	if _, err := os.Stat(installed); err == nil {
		return Lima{path: installed, engine: engine}, nil
	}
	path, err := exec.LookPath("limactl")
	// Point at Lima's installation where limactl is missing
	if err != nil {
		return Lima{}, ErrNoLima
	}
	return Lima{path: path, engine: engine}, nil
}

// Engine is the VM type for this host system, QEMU with KVM on Linux, Apple's vz on macOS, hcs on Windows, unless O3S_ENGINE names another.
func Engine() (string, error) {
	// Take the engine the user names
	if engine := os.Getenv("O3S_ENGINE"); engine != "" {
		// Turn away an engine o3s has no VM for
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

// Name is the name of a home's VM, after the same hash as its state folder.
func Name(dir string) string {
	return "o3s-" + paths.ID(dir)
}

// Up creates a home's VM from the template where it is missing and starts it, its progress on out.
func (l Lima) Up(ctx context.Context, name string, out io.Writer) error {
	names, err := l.output(ctx, "list", "--quiet")
	// Hand back a Lima that fails to list its VMs
	if err != nil {
		return err
	}
	// Start the VM the home already has
	if slices.Contains(strings.Fields(names), name) {
		return l.run(ctx, nil, out, out, "start", name)
	}
	args := []string{"start", "--name=" + name, "--vm-type=" + l.engine}
	// Name a DNS server, which the hcs driver needs set explicitly (lima website/content/en/docs/config/vmtype/hcs.md, lima-vm/lima#5662)
	if l.engine == "hcs" {
		args = append(args, "--dns=8.8.8.8")
	}
	return l.run(ctx, bytes.NewReader(template), out, out, append(args, "-")...)
}

// Exec runs a command in a home's VM, with the given streams, and hands back the command's failure.
func (l Lima) Exec(ctx context.Context, name string, in io.Reader, out, errOut io.Writer, args ...string) error {
	return l.run(ctx, in, out, errOut, append([]string{"shell", name}, args...)...)
}

// Delete stops and removes a home's VM, its disk included.
func (l Lima) Delete(ctx context.Context, name string, out io.Writer) error {
	return l.run(ctx, nil, out, out, "delete", "--force", name)
}

// run runs limactl in o3s's Lima home, without its terminal prompts.
func (l Lima) run(ctx context.Context, in io.Reader, out, errOut io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, l.path, append([]string{"--tty=false"}, args...)...) //nolint:gosec // o3s runs the limactl it found, with arguments it builds itself
	cmd.Env = append(os.Environ(), "LIMA_HOME="+paths.Lima())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errOut
	return cmd.Run()
}

// output runs limactl and hands back what it printed, with its error output in the failure.
func (l Lima) output(ctx context.Context, args ...string) (string, error) {
	var out, errOut bytes.Buffer
	// Hand back the error output along with the failure
	if err := l.run(ctx, nil, &out, &errOut, args...); err != nil {
		return "", fmt.Errorf("limactl %s: %w\n%s", strings.Join(args, " "), err, errOut.String())
	}
	return out.String(), nil
}
