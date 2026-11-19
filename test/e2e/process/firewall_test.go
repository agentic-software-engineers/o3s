//go:build e2e

package process_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/test/stacktest"
)

// The data a host moves through the gateway shows beside it, and a name the cage was refused shows among the blocked names.
func TestTheFirewallShowsTrafficAndBlockedNames(t *testing.T) {
	t.Setenv("O3S_TRAFFIC_INTERVAL", "1")
	_, err := stacktest.O3s(t, "firewall", "add", "github.com", "--port", strconv.Itoa(fixture.Port))
	// Stop when the host stays closed
	if err != nil {
		t.Fatalf("o3s firewall add = %v", err)
	}
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("DevcontainerUp() = %v, want the stack raised", err)
	}
	// Stop when the cage fails to reach the allowed host
	if _, err := stacktest.ContainerExec(t, raised.ContainerID, "curl", "-fsS", "-o", "/dev/null", "https://github.com"); err != nil {
		cage, _ := stacktest.ContainerExec(t, raised.ContainerID, "sh", "-c", "ip -4 addr; ip route; ip neigh; cat /etc/resolv.conf; getent hosts github.com; curl -v --max-time 5 -o /dev/null https://github.com 2>&1 | tail -15")
		gateway, _ := stacktest.Exec(t, "gateway", "sh", "-c", "ip -4 addr; ip route; iptables-save; cat /etc/dnsmasq.conf; ss -ltnp")
		t.Fatalf("curl https://github.com in the cage = %v\n--- cage:\n%s\n--- gateway:\n%s", err, cage, gateway)
	}
	// Ask for a name the policy leaves out, which the gateway refuses
	_, _ = stacktest.ContainerExec(t, raised.ContainerID, "getent", "hosts", fixture.RefusedName)

	shown := waitForO3s(t, []string{"firewall"}, func(out string) bool {
		return strings.Contains(out, "github.com") && !strings.Contains(firstLine(out, "github.com"), " – ") && !strings.Contains(firstLine(out, "github.com"), " 0 B ") && strings.Contains(out, fixture.RefusedName)
	})

	// Report an allowed host whose data is missing
	if !strings.Contains(firstLine(shown, "github.com"), " ago") {
		t.Errorf("o3s firewall = %q, want github.com with its data and when it last moved any", shown)
	}
	// Report a refused name missing from the blocked names
	if !strings.Contains(shown, fixture.RefusedName) {
		t.Errorf("o3s firewall = %q, want %s among the blocked names", shown, fixture.RefusedName)
	}
}

// waitForO3s runs o3s until its output holds or a few intervals pass, and hands back its last output.
func waitForO3s(t *testing.T, args []string, holds func(string) bool) string {
	t.Helper()
	var out string
	// Run the command once a second, a few times over
	for range 20 {
		got, err := stacktest.O3s(t, args...)
		out = got
		// Hand back the output once it holds
		if err == nil && holds(out) {
			return out
		}
		time.Sleep(time.Second)
	}
	return out
}

// firstLine hands back the first line of an output that names a host.
func firstLine(out, host string) string {
	// Look at every line
	for line := range strings.SplitSeq(out, "\n") {
		// Take the line that starts with the host
		if strings.HasPrefix(line, host+" ") {
			return line
		}
	}
	return ""
}

// A host allowed for a set time is reachable until its end and refused after it, without a restart in between.
func TestATimedHostClosesAtItsEnd(t *testing.T) {
	opened := time.Now()
	_, err := stacktest.O3s(t, "firewall", "add", "github.com", "--port", strconv.Itoa(fixture.Port), "--for", "90s")
	// Stop when the host stays closed
	if err != nil {
		t.Fatalf("o3s firewall add --for 90s = %v", err)
	}
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("DevcontainerUp() = %v, want the stack raised", err)
	}

	_, before := stacktest.ContainerExec(t, raised.ContainerID, "curl", "-fsS", "--max-time", "10", "-o", "/dev/null", "https://github.com")
	time.Sleep(time.Until(opened.Add(95 * time.Second)))
	_, after := stacktest.ContainerExec(t, raised.ContainerID, "curl", "-fsS", "--max-time", "10", "-o", "/dev/null", "https://github.com")

	// Report a host the cage failed to reach while it was open
	if before != nil {
		t.Errorf("curl https://github.com before the end = %v, want it reached", before)
	}
	// Report a host the cage still reached after its end
	if after == nil {
		t.Errorf("curl https://github.com after the end succeeded, want it refused")
	}
}
