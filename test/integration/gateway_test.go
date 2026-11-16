//go:build integration

// Package integration_test covers one real component of o3s at a time, built from this branch.
package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hansehart/o3s/test/stacktest"
)

// An empty policy raises a gateway that reports healthy.
func TestTheGatewayComesUpHealthyOnAnEmptyPolicy(t *testing.T) {
	stacktest.AnEmptyPolicy(t)

	err := stacktest.Up(t, "gateway")

	// Report a gateway that fails to report healthy
	if err != nil {
		t.Errorf("Up(gateway) on an empty policy = %v, want the gateway healthy", err)
	}
}

// On an empty policy the gateway forwards replies alone and rejects every new connection.
func TestTheGatewayRejectsEveryNewConnectionOnAnEmptyPolicy(t *testing.T) {
	stacktest.AnEmptyPolicy(t)
	// Stop when the gateway fails to come up
	if err := stacktest.Up(t, "gateway"); err != nil {
		t.Fatalf("Setup: Up(gateway): %v", err)
	}

	out, err := stacktest.Exec(t, "gateway", "iptables", "-S", "FORWARD")

	// Stop when reading the rules fails
	if err != nil {
		t.Fatalf("Exec(gateway, iptables -S FORWARD) = %v", err)
	}
	rules := strings.Split(strings.TrimSpace(out), "\n")
	// Check every rule in the chain
	for _, rule := range rules {
		// Flag an accepting rule that admits new connections
		if strings.HasSuffix(rule, "-j ACCEPT") && !strings.Contains(rule, "--ctstate") {
			t.Errorf("FORWARD accepts %q, want replies alone accepted", rule)
		}
	}
	// Require the chain to close with a reject
	if got, want := rules[len(rules)-1], "-A FORWARD -j REJECT --reject-with icmp-admin-prohibited"; got != want {
		t.Errorf("last FORWARD rule = %q, want %q", got, want)
	}
}

// The gateway opens every port the home's policy names for a host.
func TestTheGatewayOpensThePortsTheHomesPolicyNames(t *testing.T) {
	stacktest.AHomePolicy(t, `{"hosts": [{"name": "example.com", "ports": [443, 80], "by": ["user"]}]}`)
	// Stop when the gateway fails to come up
	if err := stacktest.Up(t, "gateway"); err != nil {
		t.Fatalf("Setup: Up(gateway): %v", err)
	}

	out, err := stacktest.Exec(t, "gateway", "iptables", "-S", "FORWARD")

	// Stop when reading the rules fails
	if err != nil {
		t.Fatalf("Exec(gateway, iptables -S FORWARD) = %v", err)
	}
	// Require an accepting rule for every port the policy names
	for _, port := range []string{"443", "80"} {
		// Flag a named port the chain leaves closed
		if !acceptsPort(out, port) {
			t.Errorf("FORWARD accepts nothing on port %s from its set, rules:\n%s", port, out)
		}
	}
}

// acceptsPort reports whether a chain holds a rule that accepts a port for the addresses in its set.
func acceptsPort(rules, port string) bool {
	// Look for one rule that names the port's set, the port and the accept
	for rule := range strings.SplitSeq(rules, "\n") {
		// Report the first rule that opens the port
		if strings.Contains(rule, "--match-set allowed-p"+port+" dst") &&
			strings.Contains(rule, "--dport "+port+" ") &&
			strings.HasSuffix(rule, "-j ACCEPT") {
			return true
		}
	}
	return false
}

// The gateway counts every host of the policy on its own, in both directions.
func TestTheGatewayCountsEveryHostOnItsOwn(t *testing.T) {
	stacktest.AHomePolicy(t, `{"hosts": [{"name": "example.com", "ports": [443], "by": ["user"]}, {"name": "192.0.2.0/24", "ports": [22], "by": ["user"]}]}`)
	// Stop when the gateway fails to come up
	if err := stacktest.Up(t, "gateway"); err != nil {
		t.Fatalf("Setup: Up(gateway): %v", err)
	}

	out, err := stacktest.Exec(t, "gateway", "iptables", "-S", "O3S_ACCT")

	// Stop when reading the rules fails
	if err != nil {
		t.Fatalf("Exec(gateway, iptables -S O3S_ACCT) = %v", err)
	}
	// Require a rule each way for every host
	for _, host := range []string{"example.com", "192.0.2.0/24"} {
		// Check both directions of the host's traffic
		for _, way := range []string{"dst", "src"} {
			// Flag a direction the chain leaves uncounted
			if !countsHost(out, host, way) {
				t.Errorf("O3S_ACCT counts no %s traffic of %s, rules:\n%s", way, host, out)
			}
		}
	}
}

// The gateway records the bytes a host moved and when it last moved any.
func TestTheGatewayRecordsTheTrafficOfAHost(t *testing.T) {
	t.Setenv("O3S_TRAFFIC_INTERVAL", "1")
	stacktest.AHomePolicy(t, `{"hosts": [{"name": "example.com", "ports": [443], "by": ["user"]}, {"name": "127.0.0.1", "ports": [9000], "by": ["user"]}]}`)
	// Stop when the gateway fails to come up
	if err := stacktest.Up(t, "gateway"); err != nil {
		t.Fatalf("Setup: Up(gateway): %v", err)
	}

	_, err := stacktest.Exec(t, "gateway", "sh", "-c", "nc -l -p 9000 >/dev/null & sleep 1; head -c 30000 /dev/zero | nc -w1 127.0.0.1 9000")

	// Stop when the traffic fails to flow
	if err != nil {
		t.Fatalf("Exec(gateway, send 30000 bytes) = %v", err)
	}
	got := waitFor(t, `.hosts["127.0.0.1"] | (.bytes >= 30000 and .last_seen > 0)`)
	// Report traffic the record misses
	if got != "true" {
		t.Errorf("traffic.json records 127.0.0.1 as %q, want at least 30000 bytes and a last seen time", got)
	}
}

// A name the gateway refuses to resolve lands among the blocked hosts.
func TestTheGatewayRecordsARefusedName(t *testing.T) {
	t.Setenv("O3S_TRAFFIC_INTERVAL", "1")
	stacktest.AHomePolicy(t, `{"hosts": [{"name": "example.com", "ports": [443], "by": ["user"]}]}`)
	// Stop when the gateway fails to come up
	if err := stacktest.Up(t, "gateway"); err != nil {
		t.Fatalf("Setup: Up(gateway): %v", err)
	}

	// Ask for a name the policy leaves out, which the resolver refuses
	_, _ = stacktest.Exec(t, "gateway", "nslookup", "blocked.example", "127.0.0.1")
	got := waitFor(t, `.blocked["blocked.example"].attempts >= 1`)

	// Report a refusal the record misses
	if got != "true" {
		t.Errorf("traffic.json records blocked.example as %q, want at least one attempt", got)
	}
}

// waitFor reads the gateway's traffic record until a query on it holds or a few intervals pass, and hands back its last answer.
func waitFor(t *testing.T, query string) string {
	t.Helper()
	var got string
	// Read the record once a second, a few times over
	for range 15 {
		out, err := stacktest.Exec(t, "gateway", "yq", "-p=json", "-o=json", query, "/state/traffic.json")
		got = strings.TrimSpace(out)
		// Hand back the answer once the record holds
		if err == nil && got == "true" {
			return got
		}
		time.Sleep(time.Second)
	}
	return got
}

// countsHost reports whether a chain holds a rule that counts a host's traffic in one direction.
func countsHost(rules, host, way string) bool {
	// Look for the rule that names the host and the direction
	for rule := range strings.SplitSeq(rules, "\n") {
		// Report the first rule that counts the host that way
		if strings.Contains(rule, " "+way+" ") && strings.Contains(rule, `--comment "`+host+`"`) {
			return true
		}
	}
	return false
}
