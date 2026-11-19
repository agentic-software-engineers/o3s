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

// The kernel refuses the cage a change of its default route.
func TestTheCageIsRefusedARouteChange(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}
	route, err := stacktest.ContainerExec(t, raised.ContainerID, "ip", "route", "show", "default")
	fields := strings.Fields(route)
	// Stop when the cage's default route stays unread
	if err != nil || len(fields) < 3 {
		t.Fatalf("Setup: ContainerExec(cage, ip route show default) = %q, %v", route, err)
	}

	_, err = stacktest.ContainerExec(t, raised.ContainerID, "ip", "route", "replace", "default", "via", fields[2])

	// Require the kernel to refuse the change
	if err == nil {
		t.Errorf("ip route replace default via %s in the cage succeeded, want it refused", fields[2])
	}
}

// The cage routes through the address Docker hands the cage network, which the gateway answers on.
func TestTheCageRoutesThroughTheCageNetworksRouteAddress(t *testing.T) {
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("Setup: DevcontainerUp() = %v, want the stack raised", err)
	}
	address, err := stacktest.CageRoute(t)
	// Stop when the route address stays unread
	if err != nil || address == "" {
		t.Fatalf("Setup: CageRoute() = %q, %v", address, err)
	}

	route, err := stacktest.ContainerExec(t, raised.ContainerID, "ip", "route", "show", "default")

	// Stop when reading the route fails
	if err != nil {
		t.Fatalf("ContainerExec(cage, ip route show default) = %v", err)
	}
	// Require the default route to lead to the route address
	if got, want := strings.TrimSpace(route), "default via "+address; !strings.HasPrefix(got, want) {
		t.Errorf("cage default route = %q, want it to start with %q", got, want)
	}
}
