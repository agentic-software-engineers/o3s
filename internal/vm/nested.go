package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrNoNestedKVM marks a host whose engine runs no VM inside the home's VM, which the nested isolation needs.
var ErrNoNestedKVM = errors.New("this machine runs no VM inside the home's VM, which the nested isolation needs")

var (
	// nestedParams are where the KVM modules say whether they run a VM inside a VM.
	nestedParams = []string{"/sys/module/kvm_intel/parameters/nested", "/sys/module/kvm_amd/parameters/nested"}
	// macOSRelease names the macOS release this host runs.
	macOSRelease = func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
		return strings.TrimSpace(string(out)), err
	}
)

// macOSNests is the first macOS release whose virtualization runs a VM inside a VM.
const macOSNests = 15

// Nests names what this host lacks to run a VM inside the home's VM, before anything is written: KVM with nesting on
// for the qemu engine, macOS 15 or later for vz.
func (l Lima) Nests() error {
	// Read each engine's own condition
	switch l.engine {
	case "qemu":
		return kvmNests()
	case "vz":
		return macNests()
	default:
		return fmt.Errorf("%w, and the %s engine offers it in a later release", ErrNoNestedKVM, l.engine)
	}
}

// kvmNests reads the loaded KVM module's nested parameter, as the VM manager does before it starts a VM.
func kvmNests() error {
	// Take the first loaded module's word on nesting
	for _, path := range nestedParams {
		value, err := os.ReadFile(path)
		// Move on past a module this host leaves unloaded
		if err != nil {
			continue
		}
		module := filepath.Base(filepath.Dir(filepath.Dir(path)))
		// Pass a module that turns nesting on
		if v := strings.TrimSpace(string(value)); v == "Y" || v == "1" {
			return nil
		}
		return fmt.Errorf("%w, turn it on as in: sudo modprobe -r %s && sudo modprobe %s nested=1", ErrNoNestedKVM, module, module)
	}
	return fmt.Errorf("%w, load KVM first: neither kvm_intel nor kvm_amd is loaded", ErrNoNestedKVM)
}

// macNests reads the macOS release, which runs a VM inside a VM from release 15 on, on a chip that offers it.
func macNests() error {
	release, err := macOSRelease()
	// Name a release this host keeps to itself
	if err != nil {
		return fmt.Errorf("%w, its macOS release is unreadable: %w", ErrNoNestedKVM, err)
	}
	major, err := strconv.Atoi(strings.SplitN(release, ".", 2)[0])
	// Refuse a release before the first that nests, and one that reads as no release
	if err != nil || major < macOSNests {
		return fmt.Errorf("%w, it needs macOS %d or later, this host runs %s", ErrNoNestedKVM, macOSNests, release)
	}
	return nil
}
