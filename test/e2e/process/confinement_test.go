//go:build e2e

// Package process_test covers the whole o3s stack raised straight on a real host, the way the editor raises it.
package process_test

import (
	"strings"
	"testing"

	"github.com/hansehart/o3s/test/stacktest"
)

// A host whose kernel lost the cage's profile still raises a cage the kernel confines.
func TestTheCageIsConfinedAfterTheKernelLosesTheProfile(t *testing.T) {
	stacktest.AHostThatLostTheProfile(t)

	raised, err := stacktest.DevcontainerUp(t)

	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("DevcontainerUp() = %v, want the stack raised", err)
	}
	out, err := stacktest.ContainerExec(t, raised.ContainerID, "cat", "/proc/self/attr/current")
	// Stop when reading the confinement fails
	if err != nil {
		t.Fatalf("ContainerExec(cage, cat /proc/self/attr/current) = %v", err)
	}
	// Require the cage to run under its profile in enforce mode
	if got, want := strings.TrimSpace(out), "o3s-cage (enforce)"; got != want {
		t.Errorf("cage confinement = %q, want %q", got, want)
	}
}

// The cage drops NET_ADMIN, which leaves its routes, interfaces and firewall to the network it sits on.
func TestTheCageDropsNetAdmin(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}

	status, err := stacktest.ContainerExec(t, raised.ContainerID, "cat", "/proc/1/status")

	// Stop when reading the capabilities fails
	if err != nil {
		t.Fatalf("ContainerExec(cage, cat /proc/1/status) = %v", err)
	}
	held, err := stacktest.Holds(status, stacktest.CapNetAdmin)
	// Stop when the capability set stays unread
	if err != nil {
		t.Fatalf("Holds(cage status, CAP_NET_ADMIN) = %v", err)
	}
	// Require the cage to have dropped NET_ADMIN
	if held {
		t.Errorf("cage holds CAP_NET_ADMIN, want it dropped")
	}
}

// The cage holds the loopback alone, so the host's sockets are its one way out.
func TestTheCageHoldsTheLoopbackAlone(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}

	links, err := stacktest.ContainerExec(t, raised.ContainerID, "ls", "/sys/class/net")

	// Require the loopback as the cage's one interface
	if err != nil || strings.TrimSpace(links) != "lo" {
		t.Errorf("cage interfaces = %q, %v, want lo alone", links, err)
	}
}

// A direct connection from the cage to a public address stays refused, past any proxy.
func TestTheCageIsRefusedADirectConnection(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}

	_, err = stacktest.ContainerExec(t, raised.ContainerID, "curl", "--noproxy", "*", "--connect-timeout", "3", "-s", "-o", "/dev/null", "https://1.1.1.1")

	// Require the connection refused
	if err == nil {
		t.Error("a direct connection from the cage to 1.1.1.1 went through, want it refused")
	}
}

// The cage reads the CPUs' thermal throttle counters as denied, as the runtime's mask would.
func TestTheCageDeniesTheThermalThrottleCounters(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}
	dir, err := stacktest.ContainerExec(t, raised.ContainerID, "sh", "-c", "ls -d /sys/devices/system/cpu/cpu*/thermal_throttle | head -n1")
	// Skip a host whose CPUs lack thermal throttle counters
	if err != nil || strings.TrimSpace(dir) == "" {
		t.Skipf("the host exposes no thermal_throttle folder: %v", err)
	}

	out, err := stacktest.ContainerExecAs(t, raised.ContainerID, "ubuntu", "sh", "-c", "cat "+strings.TrimSpace(dir)+"/*")

	// Require the counters to read as denied
	if err == nil {
		t.Errorf("cat %s/* as ubuntu = %q, want it denied", strings.TrimSpace(dir), out)
	}
}

// mountProcElsewhere mounts a procfs outside /proc from a fresh user, mount and pid namespace.
const mountProcElsewhere = "mkdir -p /tmp/p && unshare -Urmpf mount -t proc p /tmp/p"

// The dev user's mount of a procfs outside /proc is denied.
func TestTheDevUserIsDeniedAProcMountElsewhere(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}

	out, err := stacktest.ContainerExecAs(t, raised.ContainerID, "ubuntu", "sh", "-c", mountProcElsewhere)

	// Require the mount to be denied
	if err == nil {
		t.Errorf("%s as ubuntu = %q, want it denied", mountProcElsewhere, out)
	}
}

// Root's mount of a procfs outside /proc is denied.
func TestRootIsDeniedAProcMountElsewhere(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}

	out, err := stacktest.ContainerExec(t, raised.ContainerID, "sh", "-c", mountProcElsewhere)

	// Require the mount to be denied
	if err == nil {
		t.Errorf("%s as root = %q, want it denied", mountProcElsewhere, out)
	}
}

// A procfs on another folder named proc keeps the masked files denied.
func TestAProcOnAnotherProcFolderKeepsTheMaskedFilesDenied(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}
	script := "mkdir -p /tmp/x/proc && unshare -Urmpf sh -c 'mount -t proc p /tmp/x/proc && cat /tmp/x/proc/timer_list'"

	out, err := stacktest.ContainerExecAs(t, raised.ContainerID, "ubuntu", "sh", "-c", script)

	// Require the masked file to read as denied
	if err == nil {
		t.Errorf("%s as ubuntu = %q, want the read denied", script, out)
	}
}

// The nested engine runs a container, whose procfs it mounts under the profile.
func TestTheNestedEngineRunsAContainer(t *testing.T) {
	stacktest.ANestedEngine(t)
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}
	// Build a one-file image from the cage's static yq, so the container builds free of a registry
	if out, err := stacktest.ContainerExecAs(t, raised.ContainerID, "ubuntu", "sh", "-c", "for _ in $(seq 30); do docker info >/dev/null 2>&1 && break; sleep 1; done; tar -C /usr/local/bin -cf - yq | docker import - o3s-probe"); err != nil {
		t.Fatalf("Setup: import the probe image = %v\n%s", err, out)
	}

	out, err := stacktest.ContainerExecAs(t, raised.ContainerID, "ubuntu", "docker", "run", "--rm", "o3s-probe", "/yq", "--version")

	// Require the nested container to run
	if err != nil || !strings.Contains(out, "yq") {
		t.Errorf("docker run o3s-probe /yq --version = %q, %v, want yq's version", out, err)
	}
}
