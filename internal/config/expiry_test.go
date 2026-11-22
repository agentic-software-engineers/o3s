package config_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
)

// now is the moment every expiry test looks at the allowlist.
var now = fixture.Now

// at writes the moment a span away from now in the shape the allowlist keeps.
func at(span time.Duration) string {
	return now.Add(span).Format(time.RFC3339)
}

// A new host keeps the end time the change gives it.
func TestAllowGivesANewHostItsEndTime(t *testing.T) {
	var c config.Config

	_, err := c.Allow(config.Change{Name: fixture.Host, Expires: fixture.InAnHourText})

	// Stop when the host is turned away
	if err != nil {
		t.Fatalf("Allow(%s) = %v, want it allowed", fixture.Host, err)
	}
	// Report an end time the host lost
	if got := c.Hosts[0].Expires; got != fixture.InAnHourText {
		t.Errorf("Expires = %q, want %s", got, fixture.InAnHourText)
	}
}

// A new end time takes the place of the old one and reports the change, and a change that lacks one keeps it.
func TestAllowRenewsTheEndTimeAndKeepsItOtherwise(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: fixture.Host, Ports: []int{fixture.Port}, By: []string{"user"}, Expires: fixture.InAnHourText})

	diffs, err := c.Allow(config.Change{Name: fixture.Host, Expires: fixture.InTwoHoursText})

	// Stop when the renewal is turned away
	if err != nil {
		t.Fatalf("Allow(renew) = %v", err)
	}
	want := []config.Diff{{Field: "expires", Old: fixture.InAnHourText, New: fixture.InTwoHoursText}}
	// Report a renewal that names another change
	if diff := cmp.Diff(want, diffs); diff != "" {
		t.Errorf("Allow(renew) diffs (-want +got):\n%s", diff)
	}
	// Stop when a change of the ports alone fails
	if _, err := c.Allow(config.Change{Name: fixture.Host, Ports: []int{fixture.OtherPort, fixture.Port}}); err != nil {
		t.Fatalf("Allow(ports) = %v", err)
	}
	// Report an end time a change that lacked one dropped
	if got := c.Hosts[0].Expires; got != fixture.InTwoHoursText {
		t.Errorf("Expires after a change of ports = %q, want it kept", got)
	}
}

// A feature needs its host for good, so it lifts the end time the user gave the host.
func TestNeedLiftsTheEndTime(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: fixture.Host, Ports: []int{fixture.Port}, By: []string{"user"}, Expires: fixture.InAnHourText})

	err := c.Need(fixture.Store+"/"+fixture.Feature, config.Change{Name: fixture.Host, Ports: []int{fixture.Port}})

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
		config.Host{Name: fixture.Host, Ports: []int{fixture.Port}, By: []string{"user"}},
		config.Host{Name: "old." + fixture.Host, Ports: []int{fixture.Port}, By: []string{"user"}, Expires: at(-time.Hour)},
		config.Host{Name: "new." + fixture.Host, Ports: []int{fixture.Port}, By: []string{"user"}, Expires: fixture.InAnHourText},
	)

	pruned := c.Prune(now)

	// Report hosts pruned wrong
	if diff := cmp.Diff([]string{"old." + fixture.Host}, pruned); diff != "" {
		t.Errorf("Prune() (-want +got):\n%s", diff)
	}
	// Report a host that should stay
	if len(c.Hosts) != 2 || len(c.Expired(now)) != 0 {
		t.Errorf("hosts after Prune() = %+v, want the two unexpired ones", c.Hosts)
	}
}

// A host expires once its end time passes, and a host that lacks one stays open for good.
func TestExpiredComparesTheEndTime(t *testing.T) {
	tests := []struct {
		expires string
		want    bool
	}{
		{"", false},
		{at(-time.Second), true},
		{fixture.NowText, true},
		{at(time.Second), false},
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

	_, err := c.Allow(config.Change{Name: fixture.Host, Expires: "2026-11-16 19:00"})

	// Report a malformed end time taken as valid
	if !errors.Is(err, config.ErrInvalid) {
		t.Errorf("Allow(malformed expires) = %v, want %v", err, config.ErrInvalid)
	}
}

// The time left reads in its largest whole unit, expired once the end has come, and empty for a host that lacks an end.
func TestUntilWritesTheTimeLeft(t *testing.T) {
	after := func(d time.Duration) string { return fixture.Now.Add(d).Format(time.RFC3339) }
	tests := map[string]string{"": "", after(-time.Hour): "expired", after(30 * time.Second): "in 30s", after(52 * time.Minute): "in 52m", after(3 * time.Hour): "in 3h", after(72 * time.Hour): "in 3d", after(2*time.Hour - time.Second): "in 2h", after(time.Hour - 30*time.Second): "in 1h"}
	// Write every end
	for end, want := range tests {
		// Report an end written wrong
		if got := config.Until(end, fixture.Now); got != want {
			t.Errorf("Until(%q) = %q, want %q", end, got, want)
		}
	}
}
