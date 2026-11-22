package menu

import (
	"strconv"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// bothPortsTyped is both ports a host may open, as the user types them, with a space after the comma.
var bothPortsTyped = strconv.Itoa(fixture.OtherPort) + ", " + strconv.Itoa(fixture.Port)

// Every check reads a line that holds in plain words, and names what is wrong with one that breaks a rule.
func TestChecksReadWhatHoldsAndNameWhatBreaks(t *testing.T) {
	tests := []struct {
		name          string
		check         func(string) (string, error)
		good, reading string
		bad           []string
	}{
		{"hosts", checkHosts, fixture.Host + " " + fixture.OtherWildcard, fixture.Host + " and every name below " + fixture.OtherHost, []string{"", "bad host!"}},
		{"ports", checkPorts, bothPortsTyped, "ports " + bothPortsTyped, []string{"", strconv.Itoa(fixture.BadPort), "ssh"}},
		{"duration", checkFor, fixture.Duration, "open for 90 minutes", []string{"soon", "0", "-5m"}},
		{"secret name", checkSecretName, fixture.Secret, "reads as $" + fixture.Secret, []string{"lower_case"}},
		{"store", checkStore, fixture.Store, "reads as a store", []string{"Registry.example.com/Features"}},
		{"repositories", checkRepos, fixture.Repo, "git@github.com:" + fixture.Repo + ".git", []string{"", "not a repo"}},
		{"typed name", checkName(fixture.Repo), fixture.Repo, "matches", []string{fixture.OtherRepo}},
	}
	// Check every check
	for _, tt := range tests {
		got, err := tt.check(tt.good)

		// Report a good line turned away or read otherwise
		if err != nil || got != tt.reading {
			t.Errorf("%s: check(%q) = %q, %v, want %q", tt.name, tt.good, got, err, tt.reading)
		}
		// Report a bad line let through
		for _, bad := range tt.bad {
			// Report a line that breaks the rule yet holds
			if _, err := tt.check(bad); err == nil {
				t.Errorf("%s: check(%q) = nil, want the rule named", tt.name, bad)
			}
		}
	}
}

// A line that fails to read as a store is turned away with an example of the shape a store takes.
func TestStoreCheckNamesTheShapeItWants(t *testing.T) {
	_, err := checkStore("not a store")

	// Require the shape and the example
	if err == nil || err.Error() != `"not a store" reads as no store, as in ghcr.io/devcontainers/features` {
		t.Errorf("checkStore = %v", err)
	}
}
