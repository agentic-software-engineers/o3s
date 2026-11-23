//go:build e2e

// Package vm_test covers the VM an o3s home lives in, booted on a real host through the o3s CLI alone.
package vm_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hansehart/o3s/test/stacktest"
)

// o3s init makes the home inside its VM, which boots the pinned Ubuntu, keeps to its own filesystems, confines the cage
// with AppArmor, stops, and goes away with o3s cage rm --yes.
//
// The subtests share one VM in order, since every boot takes minutes.
func TestTheHomesVM(t *testing.T) {
	dir := stacktest.AVMHost(t)
	out, err := stacktest.O3sIn(t, dir, "init")
	// Stop when the home fails to come up in its VM
	if err != nil {
		t.Fatalf("o3s init = %v, want the home made inside its VM\n%s", err, out)
	}
	t.Cleanup(func() { _, _ = stacktest.O3sIn(t, dir, "cage", "rm", "--yes") })
	stacktest.ABranchCageIn(t, dir)

	t.Run("leaves the host folder its ID alone", func(t *testing.T) {
		entries, err := os.ReadDir(dir)
		// Require the one entry that finds the VM
		if err != nil || len(entries) != 1 || entries[0].Name() != ".o3s" {
			t.Errorf("host folder = %v, %v, want .o3s alone", entries, err)
		}
	})

	t.Run("holds the home inside the VM", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "cat", "o3s/o3s.json")
		// Require the home's settings in the VM
		if err != nil || !strings.Contains(out, `"hosts"`) {
			t.Errorf("o3s cage shell --vm -- cat o3s/o3s.json = %v\n%s\nwant the home's settings", err, out)
		}
	})

	t.Run("boots the pinned Ubuntu", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "cat", "/etc/os-release")
		// Require the release the template pins
		if err != nil || !strings.Contains(out, `VERSION_ID="26.04"`) {
			t.Errorf("o3s cage shell --vm -- cat /etc/os-release = %v\n%s\nwant Ubuntu 26.04", err, out)
		}
	})

	t.Run("keeps to its own filesystems", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "mount")
		// Stop when the mounts stay unread
		if err != nil {
			t.Fatalf("o3s cage shell --vm -- mount = %v\n%s", err, out)
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
		out, err := stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "cat", "/sys/module/apparmor/parameters/enabled")
		// Require the kernel to enforce AppArmor, which the cage's profile needs
		if err != nil || strings.TrimSpace(out) != "Y" {
			t.Errorf("o3s cage shell --vm -- cat /sys/module/apparmor/parameters/enabled = %v, %q, want Y", err, out)
		}
	})

	t.Run("confines the cage with its own profile", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "sh", "-c", `cd "$HOME/o3s" && "$HOME/.local/share/o3s/devcontainers/bin/devcontainer" up --workspace-folder .`)
		// Stop when the stack fails to come up in the VM
		if err != nil {
			t.Fatalf("devcontainer up in the VM = %v\n%s", err, out)
		}
		id := regexp.MustCompile(`"containerId":"([0-9a-f]+)"`).FindStringSubmatch(out)
		// Stop when the CLI's output lacks a cage
		if id == nil {
			t.Fatalf("devcontainer up in the VM names no container\n%s", out)
		}
		out, err = stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "docker", "exec", id[1], "cat", "/proc/self/attr/current")
		// Require the cage to run under its profile in enforce mode
		if err != nil || strings.TrimSpace(out) != "o3s-cage (enforce)" {
			t.Errorf("cage confinement = %v, %q, want o3s-cage (enforce)", err, out)
		}
		out, err = stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "docker", "exec", id[1], "cat", "/proc/1/status")
		held, capErr := stacktest.Holds(out, stacktest.CapNetAdmin)
		// Require the cage to have dropped NET_ADMIN, which leaves its routes to the network it sits on
		if err != nil || capErr != nil || held {
			t.Errorf("cage holds CAP_NET_ADMIN = %t, %v, %v, want it dropped", held, err, capErr)
		}
		out, err = stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "docker", "exec", id[1], "sh", "-c", `ip route replace default via "$(ip route show default | awk '{print $3}')"`)
		// Require the kernel to refuse the cage a change of its default route
		if err == nil {
			t.Errorf("ip route replace default in the cage succeeded\n%s\nwant it refused", out)
		}
		out, err = stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "docker", "ps", "--format", "{{.Names}}")
		// Require the cage alone, its way out held by the host process
		if err != nil || !strings.Contains(out, "cage") || strings.Contains(out, "gateway") {
			t.Errorf("docker ps in the VM = %v\n%s\nwant the cage running alone", err, out)
		}
	})

	t.Run("stops and starts again", func(t *testing.T) {
		// Stop when the VM fails to stop
		if out, err := stacktest.O3sIn(t, dir, "cage", "stop"); err != nil {
			t.Fatalf("o3s cage stop = %v\n%s", err, out)
		}
		// Stop when the stopped VM fails to start again
		if out, err := stacktest.O3sIn(t, dir, "cage", "start"); err != nil {
			t.Fatalf("o3s cage start after stop = %v\n%s", err, out)
		}
		// Require the restarted VM to run commands
		if out, err := stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "true"); err != nil {
			t.Errorf("o3s cage shell --vm -- true after a restart = %v\n%s", err, out)
		}
	})

	t.Run("goes away with o3s cage rm --yes", func(t *testing.T) {
		out, err := stacktest.O3sIn(t, dir, "cage", "rm", "--yes")
		// Stop when the VM stays
		if err != nil {
			t.Fatalf("o3s cage rm --yes = %v\n%s", err, out)
		}
		// Require the VM to be gone
		if out, err := stacktest.O3sIn(t, dir, "cage", "shell", "--vm", "--", "true"); err == nil {
			t.Errorf("o3s cage shell --vm -- true after rm = nil error, want a failure\n%s", out)
		}
	})
}
