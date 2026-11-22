package vm

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// ErrNoQEMU marks a Linux host that lacks QEMU, which the VM manager runs VMs with there.
var ErrNoQEMU = errors.New("this machine lacks QEMU, which runs the home's VM on Linux")

// Check names what this host lacks to run the engine's VMs, before anything downloads: QEMU's tools for the qemu
// engine.
func (l Lima) Check() error {
	// Leave the engines that ship with their system to the manager
	if l.engine != "qemu" {
		return nil
	}
	system := "qemu-system-" + qemuArch()
	// Name the tools the engine needs and how to install them
	for _, tool := range []string{system, "qemu-img"} {
		// Point a missing tool at QEMU's packages
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%w, install it as in: %s", ErrNoQEMU, qemuInstall(system))
		}
	}
	return nil
}

// Accelerated reports whether the engine runs VMs at the machine's own speed: always for the engines that ship with
// their system, and with KVM for qemu.
func (l Lima) Accelerated() bool {
	// Read the engines that ship with their system as accelerated
	if l.engine != "qemu" {
		return true
	}
	_, err := os.Stat("/dev/kvm")
	return err == nil
}

// qemuArch names QEMU's machine for this host's architecture.
func qemuArch() string {
	// Name the machine QEMU emulates for arm64
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

// qemuInstall names how to install QEMU: the packages known for amd64, and the tools to look for otherwise, as
// install.sh names them.
func qemuInstall(system string) string {
	// Give the command known for amd64
	if runtime.GOARCH == "amd64" {
		return "sudo apt-get install qemu-system-x86 qemu-utils ovmf"
	}
	return "your package manager's QEMU, so that " + system + " and qemu-img are on PATH"
}
