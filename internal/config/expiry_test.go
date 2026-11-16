package config_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
)

// now is the moment every expiry test looks at the allowlist.
var now = time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)

// A new host keeps the end time the change gives it.
func TestAllowGivesANewHostItsEndTime(t *testing.T) {
	var c config.Config

	_, err := c.Allow(config.Change{Name: "example.com", Expires: "2026-11-16T19:00:00Z"})

	// Stop when the host is turned away
	if err != nil {
		t.Fatalf("Allow(example.com) = %v, want it allowed", err)
	}
	// Report an end time the host lost
	if got := c.Hosts[0].Expires; got != "2026-11-16T19:00:00Z" {
		t.Errorf("Expires = %q, want 2026-11-16T19:00:00Z", got)
	}
}

// A new end time takes the place of the old one and reports the change, and a change without one keeps it.
func TestAllowRenewsTheEndTimeAndKeepsItOtherwise(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: "example.com", Ports: []int{443}, By: []string{"user"}, Expires: "2026-11-16T19:00:00Z"})

	diffs, err := c.Allow(config.Change{Name: "example.com", Expires: "2026-11-16T20:00:00Z"})

	// Stop when the renewal is turned away
	if err != nil {
		t.Fatalf("Allow(renew) = %v", err)
	}
	want := []config.Diff{{Field: "expires", Old: "2026-11-16T19:00:00Z", New: "2026-11-16T20:00:00Z"}}
	// Report a renewal that names another change
	if diff := cmp.Diff(want, diffs); diff != "" {
		t.Errorf("Allow(renew) diffs (-want +got):\n%s", diff)
	}
	// Stop when a change of the ports alone fails
	if _, err := c.Allow(config.Change{Name: "example.com", Ports: []int{22, 443}}); err != nil {
		t.Fatalf("Allow(ports) = %v", err)
	}
	// Report an end time a change without one dropped
	if got := c.Hosts[0].Expires; got != "2026-11-16T20:00:00Z" {
		t.Errorf("Expires after a change of ports = %q, want it kept", got)
	}
}

// A feature needs its host for good, so it lifts the end time the user gave the host.
func TestNeedLiftsTheEndTime(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: "example.com", Ports: []int{443}, By: []string{"user"}, Expires: "2026-11-16T19:00:00Z"})

	err := c.Need("registry.example.com/features/tool", config.Change{Name: "example.com", Ports: []int{443}})

	// Stop when the feature is turned away
	if err != nil {
		t.Fatalf("Need() = %v", err)
	}
	// Report an end time the feature left on its host
	if got := c.Hosts[0].Expires; got != "" {
		t.Errorf("Expires = %q, want none once a feature needs the host", got)
	}
}

// The hosts whose end time passed are the expired ones, and prune takes them off.
func TestPruneTakesOffTheExpiredHosts(t *testing.T) {
	c := aConfigThatAllows(
		config.Host{Name: "example.com", Ports: []int{443}, By: []string{"user"}},
		config.Host{Name: "old.example.com", Ports: []int{443}, By: []string{"user"}, Expires: "2026-11-16T17:00:00Z"},
		config.Host{Name: "new.example.com", Ports: []int{443}, By: []string{"user"}, Expires: "2026-11-16T19:00:00Z"},
	)

	pruned := c.Prune(now)

	// Report hosts pruned wrong
	if diff := cmp.Diff([]string{"old.example.com"}, pruned); diff != "" {
		t.Errorf("Prune() (-want +got):\n%s", diff)
	}
	// Report a host that should stay
	if len(c.Hosts) != 2 || len(c.Expired(now)) != 0 {
		t.Errorf("hosts after Prune() = %+v, want the two unexpired ones", c.Hosts)
	}
}

// A host expires once its end time passes, and a host without one never does.
func TestExpiredComparesTheEndTime(t *testing.T) {
	tests := []struct {
		expires string
		want    bool
	}{
		{"", false},
		{"2026-11-16T17:59:59Z", true},
		{"2026-11-16T18:00:00Z", true},
		{"2026-11-16T18:00:01Z", false},
	}
	// Judge every end time
	for _, tt := range tests {
		got := config.Host{Expires: tt.expires}.Expired(now)

		// Report an end time judged wrong
		if got != tt.want {
			t.Errorf("Expired(%q) = %v, want %v", tt.expires, got, tt.want)
		}
	}
}

// An end time in another shape than UTC with whole seconds is turned away.
func TestAllowTurnsAwayAMalformedEndTime(t *testing.T) {
	var c config.Config

	_, err := c.Allow(config.Change{Name: "example.com", Expires: "2026-11-16 19:00"})

	// Report a malformed end time taken as valid
	if !errors.Is(err, config.ErrInvalid) {
		t.Errorf("Allow(malformed expires) = %v, want %v", err, config.ErrInvalid)
	}
}
