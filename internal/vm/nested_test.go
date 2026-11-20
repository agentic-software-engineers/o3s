package vm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withNestedParams points the check at module parameters written into a temp folder, one file per given value, and
// restores the real paths afterwards.
func withNestedParams(t *testing.T, values map[string]string) {
	t.Helper()
	dir := t.TempDir()
	saved := nestedParams
	t.Cleanup(func() { nestedParams = saved })
	nestedParams = nil
	// Write each module's parameter where the check reads it, and list the paths of the modules left unloaded too
	for _, module := range []string{"kvm_intel", "kvm_amd"} {
		path := filepath.Join(dir, module, "parameters", "nested")
		nestedParams = append(nestedParams, path)
		value, ok := values[module]
		// Leave a module without a value unloaded
		if !ok {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// A KVM module that turns nesting on, as Y or as 1, lets the qemu engine run a VM inside the home's VM.
func TestNestsWhereKVMTurnsNestingOn(t *testing.T) {
	for _, value := range []string{"Y", "1"} {
		withNestedParams(t, map[string]string{"kvm_amd": value})

		err := Lima{engine: "qemu"}.Nests()

		// Require the check to pass
		if err != nil {
			t.Errorf("nested=%s: Nests() = %v, want nil", value, err)
		}
	}
}

// A KVM module that turns nesting off refuses the nested isolation and names how to turn it on for that module.
func TestNestsRefusesWhereKVMTurnsNestingOff(t *testing.T) {
	for _, value := range []string{"N", "0"} {
		withNestedParams(t, map[string]string{"kvm_intel": value})

		err := Lima{engine: "qemu"}.Nests()

		// Require the refusal, naming the module to reload
		if !errors.Is(err, ErrNoNestedKVM) || !strings.Contains(err.Error(), "kvm_intel nested=1") {
			t.Errorf("nested=%s: Nests() = %v, want %v naming kvm_intel nested=1", value, err, ErrNoNestedKVM)
		}
	}
}

// A host without a KVM module refuses the nested isolation.
func TestNestsRefusesWithoutKVM(t *testing.T) {
	withNestedParams(t, nil)

	err := Lima{engine: "qemu"}.Nests()

	// Require the refusal
	if !errors.Is(err, ErrNoNestedKVM) {
		t.Errorf("Nests() = %v, want %v", err, ErrNoNestedKVM)
	}
}

// vz runs a VM inside a VM from macOS 15 on, and refuses on older releases.
func TestNestsFollowsTheMacOSRelease(t *testing.T) {
	for release, nests := range map[string]bool{"15.0": true, "26.1": true, "14.7.2": false} {
		saved := macOSRelease
		macOSRelease = func() (string, error) { return release, nil }

		err := Lima{engine: "vz"}.Nests()

		macOSRelease = saved
		// Require a pass on the releases that nest and the refusal on the others
		if nests != (err == nil) || (!nests && !errors.Is(err, ErrNoNestedKVM)) {
			t.Errorf("macOS %s: Nests() = %v, want nests %v", release, err, nests)
		}
	}
}

// An engine that offers no VM inside a VM yet refuses the nested isolation.
func TestNestsRefusesOnTheOtherEngines(t *testing.T) {
	err := Lima{engine: "hcs"}.Nests()

	// Require the refusal
	if !errors.Is(err, ErrNoNestedKVM) {
		t.Errorf("Nests() = %v, want %v", err, ErrNoNestedKVM)
	}
}
