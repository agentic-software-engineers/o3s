package project

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// Every form ghq and gh take reads as the repository it names, the short ones cloned over SSH.
func TestParseReadsEveryForm(t *testing.T) {
	tests := []struct {
		arg  string
		want Repository
	}{
		{"hansehart/o3s", Repository{Host: "github.com", Port: 22, Owner: "hansehart", Name: "o3s", URL: "git@github.com:hansehart/o3s.git", SSH: true}},
		{"hansehart/o3s.git", Repository{Host: "github.com", Port: 22, Owner: "hansehart", Name: "o3s", URL: "git@github.com:hansehart/o3s.git", SSH: true}},
		{"gitlab.com/group/app", Repository{Host: "gitlab.com", Port: 22, Owner: "group", Name: "app", URL: "git@gitlab.com:group/app.git", SSH: true}},
		{"git@github.com:hansehart/o3s.git", Repository{Host: "github.com", Port: 22, Owner: "hansehart", Name: "o3s", URL: "git@github.com:hansehart/o3s.git", SSH: true}},
		{"git@gitlab.com:group/sub/app.git", Repository{Host: "gitlab.com", Port: 22, Owner: "group/sub", Name: "app", URL: "git@gitlab.com:group/sub/app.git", SSH: true}},
		{"ssh://git@example.com:2222/team/app.git", Repository{Host: "example.com", Port: 2222, Owner: "team", Name: "app", URL: "ssh://git@example.com:2222/team/app.git", SSH: true}},
		{"https://github.com/hansehart/o3s", Repository{Host: "github.com", Port: 443, Owner: "hansehart", Name: "o3s", URL: "https://github.com/hansehart/o3s"}},
	}
	// Read every form
	for _, tt := range tests {
		got, err := Parse(tt.arg)
		// Report a form that fails to read
		if err != nil {
			t.Errorf("Parse(%q) = %v", tt.arg, err)
			continue
		}
		// Report a form read as another repository
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("Parse(%q) mismatch (-want +got):\n%s", tt.arg, diff)
		}
	}
}

// An argument that names no repository is turned away as invalid.
func TestParseTurnsAwayWhatNamesNoRepository(t *testing.T) {
	// Try every malformed argument
	for _, arg := range []string{"o3s", "", "a/b/c", "http://github.com/o/r", "git://github.com/o/r", "https://github.com/o3s", "owner/..", "owner/a b"} {
		// Report an argument read as a repository
		if _, err := Parse(arg); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) = %v, want %v", arg, err, ErrInvalid)
		}
	}
}

// A clone's remote names its owner/repo, and a remote that names none says so.
func TestOwnerRepoReadsTheRemote(t *testing.T) {
	tests := []struct {
		remote string
		want   string
		ok     bool
	}{
		{"git@github.com:hansehart/o3s.git\n", "hansehart/o3s", true},
		{"https://github.com/hansehart/o3s", "hansehart/o3s", true},
		{"/srv/git/app", "", false},
	}
	// Read every remote
	for _, tt := range tests {
		got, ok := OwnerRepo(tt.remote)
		// Report a remote read wrong
		if got != tt.want || ok != tt.ok {
			t.Errorf("OwnerRepo(%q) = %q, %v, want %q, %v", tt.remote, got, ok, tt.want, tt.ok)
		}
	}
}

// A project's folder sits right below the projects volume.
func TestValidNameKeepsTheFolderInTheVolume(t *testing.T) {
	// Try names that stay and names that leave
	for name, want := range map[string]bool{"app": true, "my-app.v2": true, "..": false, ".": false, "a/b": false, "": false} {
		// Report a name judged wrong
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}
