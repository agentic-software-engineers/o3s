//go:build vm

// Package e2evm_test covers the VM an o3s home lives in, booted on a real host through the o3s CLI alone, like Podman's machine suite.
package e2evm_test

import (
	"strings"
	"testing"

	"github.com/hansehart/o3s/test/stacktest"
)

// A home's VM boots the pinned Ubuntu, holds no host folder, runs AppArmor, and goes away with o3s vm rm.
//
// The subtests share one VM in order, since every boot takes minutes.
func TestTheHomesVM(t *testing.T) {
	dir := stacktest.AVMHost(t)
	out, err := stacktest.O3sIn(t, dir, "init", "--vm")
	// Stop when the VM fails to come up
	if err != nil {
		t.Fatalf("o3s init --vm = %v, want the home's VM running\n%s", err, out)
	}
	t.Cleanup(func() { _, _ = stacktest.O3sIn(t, dir, "vm", "rm") })

	t.Run("boots the pinned Ubuntu", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "vm", "ssh", "--", "cat", "/etc/os-release")
		// Require the release the template pins
		if err != nil || !strings.Contains(out, `VERSION_ID="26.04"`) {
			t.Errorf("o3s vm ssh -- cat /etc/os-release = %v\n%s\nwant Ubuntu 26.04", err, out)
		}
	})

	t.Run("sees no host folder", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "vm", "ssh", "--", "mount")
		// Stop when the mounts stay unread
		if err != nil {
			t.Fatalf("o3s vm ssh -- mount = %v\n%s", err, out)
		}
		// Require no filesystem that carries host folders into the VM
		for _, kind := range []string{"type 9p", "type virtiofs", "type fuse.sshfs", "type drvfs"} {
			// Name the host folder the VM sees
			if strings.Contains(out, kind) {
				t.Errorf("mount lists %q, want no host folder\n%s", kind, out)
			}
		}
	})

	t.Run("runs AppArmor", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "vm", "ssh", "--", "cat", "/sys/module/apparmor/parameters/enabled")
		// Require the kernel to enforce AppArmor, which the cage's profile needs
		if err != nil || strings.TrimSpace(out) != "Y" {
			t.Errorf("o3s vm ssh -- cat /sys/module/apparmor/parameters/enabled = %v, %q, want Y", err, out)
		}
	})

	t.Run("goes away with o3s vm rm", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "vm", "rm")
		// Stop when the VM stays
		if err != nil {
			t.Fatalf("o3s vm rm = %v\n%s", err, out)
		}
		// Require the VM to be gone
		if out, err := stacktest.O3sIn(t, dir, "vm", "ssh", "--", "true"); err == nil {
			t.Errorf("o3s vm ssh -- true after rm = nil error, want a failure\n%s", out)
		}
	})
}
