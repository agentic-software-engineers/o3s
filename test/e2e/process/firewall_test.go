//go:build e2e

package process_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/test/stacktest"
)

// The cage reaches a host the allowlist holds through the host process, and an unlisted host stays refused.
func TestTheCageReachesTheListedHostsAlone(t *testing.T) {
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

	_, listed := stacktest.ContainerExec(t, raised.ContainerID, "curl", "-fsS", "-o", "/dev/null", "https://github.com")
	_, unlisted := stacktest.ContainerExec(t, raised.ContainerID, "curl", "-fsS", "--max-time", "10", "-o", "/dev/null", "https://"+fixture.OtherHost)

	// Require the listed host reached and the unlisted one refused
	if listed != nil || unlisted == nil {
		t.Errorf("curl github.com = %v, curl %s = %v, want the first through and the second refused", listed, fixture.OtherHost, unlisted)
	}
}

// A host allowed for a set time is reachable until its end and refused after it, with the stack running throughout.
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
