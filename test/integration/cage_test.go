//go:build integration

// Package integration_test covers one real component of o3s at a time, built from this branch.
package integration_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/test/stacktest"
)

// listing is a policy that opens the fixture's host on the HTTPS port.
func listing() string {
	return fmt.Sprintf(`{"hosts": [{"name": %q, "ports": [%d], "by": ["user"]}]}`, fixture.Host, fixture.Port)
}

// aCage raises the cage of the test's home under the policy given.
func aCage(t *testing.T, policy string) {
	t.Helper()
	stacktest.AHomePolicy(t, policy)
	// Stop when the cage fails to come up
	if err := stacktest.Up(t, "cage"); err != nil {
		t.Fatalf("Setup: Up(cage) = %v", err)
	}
}

// The cage holds the loopback alone.
func TestTheCageHoldsTheLoopbackAlone(t *testing.T) {
	aCage(t, listing())

	links, err := stacktest.Exec(t, "cage", "ls", "/sys/class/net")

	// Require the loopback as the cage's one interface
	if err != nil || strings.TrimSpace(links) != "lo" {
		t.Errorf("cage interfaces = %q, %v, want lo alone", links, err)
	}
}

// A host the allowlist holds answers the cage through the host process.
func TestTheCageReachesAListedHost(t *testing.T) {
	aCage(t, listing())

	_, err := stacktest.Exec(t, "cage", "curl", "-fsS", "--max-time", "20", "-o", "/dev/null", "https://"+fixture.Host)

	// Require the host's answer
	if err != nil {
		t.Errorf("curl https://%s in the cage = %v, want an answer", fixture.Host, err)
	}
}

// A host the allowlist leaves out stays out of the cage's reach.
func TestTheCageIsRefusedAnUnlistedHost(t *testing.T) {
	aCage(t, listing())

	_, err := stacktest.Exec(t, "cage", "curl", "-fsS", "--max-time", "20", "-o", "/dev/null", "https://"+fixture.OtherHost)

	// Require the refusal
	if err == nil {
		t.Errorf("curl https://%s in the cage went through, want it refused", fixture.OtherHost)
	}
}

// The cage resolves a name the allowlist holds, and a name it leaves out reads as missing.
func TestTheCageResolvesTheListedNamesAlone(t *testing.T) {
	aCage(t, listing())

	_, listed := stacktest.Exec(t, "cage", "getent", "hosts", fixture.Host)
	_, unlisted := stacktest.Exec(t, "cage", "getent", "hosts", fixture.OtherHost)

	// Require the listed name resolved and the other missing
	if listed != nil || unlisted == nil {
		t.Errorf("getent %s = %v, getent %s = %v, want the first resolved and the second missing", fixture.Host, listed, fixture.OtherHost, unlisted)
	}
}

// A host added while the cage runs answers within moments, with the cage and the host process left running.
func TestTheCageFollowsAnAllowlistChange(t *testing.T) {
	aCage(t, `{"hosts": []}`)

	stacktest.AHomePolicy(t, listing())

	deadline := time.Now().Add(15 * time.Second)
	// Ask again until the host answers or the time runs out
	for {
		_, err := stacktest.Exec(t, "cage", "curl", "-fsS", "--max-time", "10", "-o", "/dev/null", "https://"+fixture.Host)
		// Stop once the host answers
		if err == nil {
			return
		}
		// Report a change that stays unapplied once the time runs out
		if time.Now().After(deadline) {
			t.Fatalf("curl https://%s after the change = %v, want an answer within 15s", fixture.Host, err)
		}
		time.Sleep(time.Second)
	}
}
