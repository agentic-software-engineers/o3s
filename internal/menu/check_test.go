package menu

import (
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// Every check reads a line that holds in plain words, and names what is wrong with one that breaks a rule.
func TestChecksReadWhatHoldsAndNameWhatBreaks(t *testing.T) {
	tests := []struct {
		name          string
		check         func(string) (string, error)
		good, reading string
		bad           []string
	}{
		{"hosts", checkHosts, fixture.Host + " " + fixture.Subdomain, "2 hosts", []string{"", "bad host!"}},
		{"ports", checkPorts, "22, 443", "ports 22, 443", []string{"", "70000", "ssh"}},
		{"duration", checkFor, "90m", "open for 90 minutes", []string{"soon", "0", "-5m"}},
		{"secret name", checkSecretName, fixture.Secret, "reads as $" + fixture.Secret, []string{"lower_case"}},
		{"store", checkStore, fixture.Store, "reads as a store", []string{"Registry.example.com/Features"}},
		{"repositories", checkRepos, fixture.Repo, "git@github.com:" + fixture.Repo + ".git", []string{"", "not a repo"}},
		{"typed name", checkName(fixture.Repo), fixture.Repo, "matches", []string{"other/name"}},
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
