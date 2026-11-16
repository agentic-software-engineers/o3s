//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hansehart/o3s/test/stacktest"
)

// The data a host moves through the gateway shows in the allowlist, and a name the cage was refused shows in the log.
func TestTheFirewallShowsTrafficAndBlockedNames(t *testing.T) {
	t.Setenv("O3S_TRAFFIC_INTERVAL", "1")
	_, err := stacktest.O3s(t, "firewall", "add", "github.com", "--port", "443")
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
		t.Fatalf("curl https://github.com in the cage = %v", err)
	}
	// Ask for a name the policy leaves out, which the gateway refuses
	_, _ = stacktest.ContainerExec(t, raised.ContainerID, "getent", "hosts", "blocked.example")

	listed := waitForO3s(t, []string{"firewall", "ls"}, func(out string) bool {
		return strings.Contains(out, "github.com") && !strings.Contains(firstLine(out, "github.com"), " – ") && !strings.Contains(firstLine(out, "github.com"), " 0 B ")
	})
	logged := waitForO3s(t, []string{"firewall", "log"}, func(out string) bool {
		return strings.Contains(out, "blocked.example")
	})

	// Report an allowed host whose data is missing
	if !strings.Contains(firstLine(listed, "github.com"), " ago") {
		t.Errorf("o3s firewall ls = %q, want github.com with its data and when it last moved any", listed)
	}
	// Report a refused name missing from the log
	if !strings.Contains(logged, "blocked.example") {
		t.Errorf("o3s firewall log = %q, want blocked.example among the blocked names", logged)
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
