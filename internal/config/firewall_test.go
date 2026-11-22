package config_test

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
)

// aConfigThatAllows hands a test a config holding the given hosts.
func aConfigThatAllows(hosts ...config.Host) config.Config {
	return config.Config{Hosts: hosts}
}

// A new host takes the default port, the header for its secret, and the user as the one who needs it.
func TestAllowGivesANewHostItsDefaults(t *testing.T) {
	var c config.Config

	diffs, err := c.Allow(config.Change{Name: fixture.Subdomain, Secret: fixture.Secret})

	// Stop when the host is turned away
	if err != nil {
		t.Fatalf("Allow(%s) = %v, want it allowed", fixture.Subdomain, err)
	}
	want := []config.Host{{Name: fixture.Subdomain, Ports: []int{fixture.Port}, Secret: fixture.Secret, Inject: []string{"header"}, By: []string{"user"}}}
	// Require the defaults on the new host
	if diff := cmp.Diff(want, c.Hosts); diff != "" {
		t.Errorf("Allow(%s) hosts (-want +got):\n%s", fixture.Subdomain, diff)
	}
	// Require a new host to report an empty list of changed fields
	if len(diffs) != 0 {
		t.Errorf("Allow(%s) diffs = %v, want none for a new host", fixture.Subdomain, diffs)
	}
}

// Ports arrive deduplicated and in order, whatever order they were given in.
func TestAllowOrdersAndDeduplicatesPorts(t *testing.T) {
	var c config.Config

	_, err := c.Allow(config.Change{Name: fixture.Host, Ports: []int{fixture.Port, fixture.OtherPort, fixture.Port}})

	// Stop when the host is turned away
	if err != nil {
		t.Fatalf("Allow(%s) = %v", fixture.Host, err)
	}
	// Require each port once, in order
	if diff := cmp.Diff([]int{fixture.OtherPort, fixture.Port}, c.Hosts[0].Ports); diff != "" {
		t.Errorf("Allow(%s) ports (-want +got):\n%s", fixture.Host, diff)
	}
}

// Updating a host replaces the fields given as a whole, keeps the rest, and reports each change.
func TestAllowUpdatesTheFieldsGivenAndKeepsTheRest(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: fixture.Subdomain, Ports: []int{fixture.Port}, Secret: fixture.Secret, Inject: []string{"header"}, By: []string{"user"}})

	diffs, err := c.Allow(config.Change{Name: fixture.Subdomain, Ports: []int{fixture.Port, 8443}})

	// Stop when the update is turned away
	if err != nil {
		t.Fatalf("Allow(%s) = %v", fixture.Subdomain, err)
	}
	want := []config.Host{{Name: fixture.Subdomain, Ports: []int{fixture.Port, 8443}, Secret: fixture.Secret, Inject: []string{"header"}, By: []string{"user"}}}
	// Require the new ports beside the kept secret and inject sites
	if diff := cmp.Diff(want, c.Hosts); diff != "" {
		t.Errorf("Allow(%s) hosts (-want +got):\n%s", fixture.Subdomain, diff)
	}
	// Require the ports, and the ports alone, in the report
	if diff := cmp.Diff([]config.Diff{{Field: "ports", Old: strconv.Itoa(fixture.Port), New: fmt.Sprintf("%d,8443", fixture.Port)}}, diffs); diff != "" {
		t.Errorf("Allow(%s) diffs (-want +got):\n%s", fixture.Subdomain, diff)
	}
}

// Updating a host a feature brought makes it the user's as well, so removing the feature keeps it.
func TestAllowAddsTheUserToAFeaturesHost(t *testing.T) {
	feature := fixture.Store + "/" + fixture.Feature
	c := aConfigThatAllows(config.Host{Name: fixture.OtherHost, Ports: []int{fixture.Port}, By: []string{feature}})

	diffs, err := c.Allow(config.Change{Name: fixture.OtherHost})

	// Stop when the update is turned away
	if err != nil {
		t.Fatalf("Allow(%s) = %v", fixture.OtherHost, err)
	}
	// Require the user beside the feature
	if diff := cmp.Diff([]string{feature, "user"}, c.Hosts[0].By); diff != "" {
		t.Errorf("Allow(%s) by (-want +got):\n%s", fixture.OtherHost, diff)
	}
	// Require the report to show who needs the host now
	if diff := cmp.Diff([]config.Diff{{Field: "by", Old: feature, New: feature + ",user"}}, diffs); diff != "" {
		t.Errorf("Allow(%s) diffs (-want +got):\n%s", fixture.OtherHost, diff)
	}
}

// Hosts follow their apex domain, the apex before its subdomains, with IPs and ranges last.
func TestAllowOrdersHostsByApex(t *testing.T) {
	var c config.Config
	names := []string{fixture.Range, fixture.OtherWildcard, fixture.Subdomain, fixture.OtherHost, fixture.Address, fixture.Host}

	// Allow every host, in an order the config has to put right
	for _, name := range names {
		// Stop when a host is turned away
		if _, err := c.Allow(config.Change{Name: name}); err != nil {
			t.Fatalf("Allow(%q) = %v", name, err)
		}
	}

	got := make([]string, 0, len(c.Hosts))
	// Read back the order the config holds
	for _, h := range c.Hosts {
		got = append(got, h.Name)
	}
	want := []string{fixture.Host, fixture.Subdomain, fixture.OtherHost, fixture.OtherWildcard, fixture.Address, fixture.Range}
	// Require the apex order
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("host order (-want +got):\n%s", diff)
	}
}

// Removing a host the user alone needs takes it off the allowlist.
func TestRemoveDropsAHostTheUserAloneNeeds(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: fixture.Host, Ports: []int{fixture.OtherPort, fixture.Port}, By: []string{"user"}})

	kept, err := c.Remove(fixture.Host)

	// Stop when the removal is turned away
	if err != nil {
		t.Fatalf("Remove(%s) = %v", fixture.Host, err)
	}
	// Require the host gone
	if len(c.Hosts) != 0 || kept != nil {
		t.Errorf("Remove(%s) = kept %v, hosts %v, want the host gone", fixture.Host, kept, c.Hosts)
	}
}

// Removing a host a feature still needs keeps it, and names the feature.
func TestRemoveKeepsAHostAFeatureStillNeeds(t *testing.T) {
	feature := fixture.Store + "/" + fixture.Feature
	c := aConfigThatAllows(config.Host{Name: fixture.Subdomain, Ports: []int{fixture.Port}, By: []string{feature, "user"}})

	kept, err := c.Remove(fixture.Subdomain)

	// Stop when the removal is turned away
	if err != nil {
		t.Fatalf("Remove(%s) = %v", fixture.Subdomain, err)
	}
	// Require the feature named as the one still in need
	if diff := cmp.Diff([]string{feature}, kept); diff != "" {
		t.Errorf("Remove(%s) kept (-want +got):\n%s", fixture.Subdomain, diff)
	}
	// Require the feature alone to hold the host
	if diff := cmp.Diff([]string{feature}, c.Hosts[0].By); diff != "" {
		t.Errorf("Remove(%s) by (-want +got):\n%s", fixture.Subdomain, diff)
	}
}

// Removing a host that features alone brought is turned away, since the features own it.
func TestRemoveTurnsAwayAHostOnlyFeaturesBrought(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: fixture.OtherHost, Ports: []int{fixture.Port}, By: []string{fixture.Store + "/" + fixture.Feature}})

	_, err := c.Remove(fixture.OtherHost)

	// Require the refusal to point at the features
	if !errors.Is(err, config.ErrOwnedByFeatures) {
		t.Errorf("Remove(%s) = %v, want ErrOwnedByFeatures", fixture.OtherHost, err)
	}
}

// Removing a host the allowlist lacks is turned away.
func TestRemoveTurnsAwayAnUnknownHost(t *testing.T) {
	var c config.Config

	_, err := c.Remove(fixture.Host)

	// Require the refusal to name the unknown host
	if !errors.Is(err, config.ErrUnknownHost) {
		t.Errorf("Remove(%s) = %v, want ErrUnknownHost", fixture.Host, err)
	}
}

// A change can open a host for good again and take its secret off, the end time, the secret and its sites gone.
func TestAllowOpensForGoodAndTakesTheSecretOff(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: fixture.Subdomain, Ports: []int{fixture.Port}, Secret: fixture.Secret, Inject: []string{"header"}, By: []string{"user"}, Expires: fixture.LongAgoText})

	diffs, err := c.Allow(config.Change{Name: fixture.Subdomain, ForGood: true, NoSecret: true})

	// Stop when the update is turned away
	if err != nil {
		t.Fatalf("Allow(%s) = %v", fixture.Subdomain, err)
	}
	want := []config.Host{{Name: fixture.Subdomain, Ports: []int{fixture.Port}, By: []string{"user"}}}
	// Require the host open for good and free of a secret
	if diff := cmp.Diff(want, c.Hosts); diff != "" {
		t.Errorf("Allow(%s) hosts (-want +got):\n%s", fixture.Subdomain, diff)
	}
	wantDiffs := []config.Diff{{Field: "secret", Old: fixture.Secret}, {Field: "inject", Old: "header"}, {Field: "expires", Old: fixture.LongAgoText}}
	// Report every field taken off
	if diff := cmp.Diff(wantDiffs, diffs); diff != "" {
		t.Errorf("Allow(%s) diffs (-want +got):\n%s", fixture.Subdomain, diff)
	}
}

// The allowlist lets a host through on a port it names, by its name or by a wildcard above it.
func TestAllowsReadsNamesAndWildcards(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: fixture.Host, Ports: []int{fixture.OtherPort}}, config.Host{Name: fixture.OtherWildcard, Ports: []int{fixture.Port}})
	tests := []struct {
		host string
		port int
		want bool
	}{
		{fixture.Host, fixture.OtherPort, true},
		{fixture.Host, fixture.Port, false},
		{"api." + fixture.OtherHost, fixture.Port, true},
		{fixture.ThirdHost, fixture.Port, false},
	}
	// Check every host and port
	for _, tt := range tests {
		// Report a host read otherwise
		if got := c.Allows(tt.host, tt.port); got != tt.want {
			t.Errorf("Allows(%s, %d) = %v, want %v", tt.host, tt.port, got, tt.want)
		}
	}
}

// A name counts as listed on any port, by its own entry or by a wildcard above it, and a wildcard leaves its apex out.
func TestListsReadsNamesAndWildcardsOnAnyPort(t *testing.T) {
	c := aConfigThatAllows(config.Host{Name: fixture.Host, Ports: []int{fixture.OtherPort}}, config.Host{Name: fixture.OtherWildcard, Ports: []int{fixture.Port}})
	tests := []struct {
		name string
		want bool
	}{
		{fixture.Host, true},
		{"api." + fixture.OtherHost, true},
		{fixture.OtherHost, false},
		{fixture.ThirdHost, false},
	}
	// Check every name
	for _, tt := range tests {
		// Report a name read otherwise
		if got := c.Lists(tt.name); got != tt.want {
			t.Errorf("Lists(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
