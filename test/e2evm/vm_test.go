//go:build vm

// Package e2evm_test covers the VM an o3s home lives in, booted on a real host through the o3s CLI alone.
package e2evm_test

import (
	"strings"
	"testing"

	"github.com/hansehart/o3s/test/stacktest"
)

// A home's VM boots the pinned Ubuntu, keeps to its own filesystems, runs AppArmor, stops, and goes away with o3s vm rm.
//
// The subtests share one VM in order, since every boot takes minutes.
func TestTheHomesVM(t *testing.T) {
	dir := stacktest.AVMHost(t)
	out, err := stacktest.O3sIn(t, dir, "vm", "start")
	// Stop when the VM fails to come up
	if err != nil {
		t.Fatalf("o3s vm start = %v, want the home's VM running\n%s", err, out)
	}
	t.Cleanup(func() { _, _ = stacktest.O3sIn(t, dir, "vm", "rm") })

	t.Run("boots the pinned Ubuntu", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "vm", "shell", "--", "cat", "/etc/os-release")
		// Require the release the template pins
		if err != nil || !strings.Contains(out, `VERSION_ID="26.04"`) {
			t.Errorf("o3s vm shell -- cat /etc/os-release = %v\n%s\nwant Ubuntu 26.04", err, out)
		}
	})

	t.Run("keeps to its own filesystems", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "vm", "shell", "--", "mount")
		// Stop when the mounts stay unread
		if err != nil {
			t.Fatalf("o3s vm shell -- mount = %v\n%s", err, out)
		}
		// Require the VM's own filesystems alone, free of any that carries host folders in
		for _, kind := range []string{"type 9p", "type virtiofs", "type fuse.sshfs", "type drvfs"} {
			// Name the host folder the VM sees
			if strings.Contains(out, kind) {
				t.Errorf("mount lists %q, want no host folder\n%s", kind, out)
			}
		}
	})

	t.Run("runs AppArmor", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "vm", "shell", "--", "cat", "/sys/module/apparmor/parameters/enabled")
		// Require the kernel to enforce AppArmor, which the cage's profile needs
		if err != nil || strings.TrimSpace(out) != "Y" {
			t.Errorf("o3s vm shell -- cat /sys/module/apparmor/parameters/enabled = %v, %q, want Y", err, out)
		}
	})

	t.Run("stops and starts again", func(t *testing.T) {
		// Stop when the VM fails to stop
		if out, err := stacktest.O3sIn(t, dir, "vm", "stop"); err != nil {
			t.Fatalf("o3s vm stop = %v\n%s", err, out)
		}
		// Stop when the stopped VM fails to start again
		if out, err := stacktest.O3sIn(t, dir, "vm", "start"); err != nil {
			t.Fatalf("o3s vm start after stop = %v\n%s", err, out)
		}
		// Require the restarted VM to run commands
		if out, err := stacktest.O3sIn(t, dir, "vm", "shell", "--", "true"); err != nil {
			t.Errorf("o3s vm shell -- true after a restart = %v\n%s", err, out)
		}
	})

	t.Run("goes away with o3s vm rm", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "vm", "rm")
		// Stop when the VM stays
		if err != nil {
			t.Fatalf("o3s vm rm = %v\n%s", err, out)
		}
		// Require the VM to be gone
		if out, err := stacktest.O3sIn(t, dir, "vm", "shell", "--", "true"); err == nil {
			t.Errorf("o3s vm shell -- true after rm = nil error, want a failure\n%s", out)
		}
	})
}
