package project

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// Every form a repository takes reads as the repository it names, the short ones cloned over SSH.
func TestParseReadsEveryForm(t *testing.T) {
	tests := []struct {
		arg  string
		want Repository
	}{
		{"torvalds/linux", Repository{Host: "github.com", Port: 22, Owner: "torvalds", Name: "linux", URL: "git@github.com:torvalds/linux.git", SSH: true}},
		{"torvalds/linux.git", Repository{Host: "github.com", Port: 22, Owner: "torvalds", Name: "linux", URL: "git@github.com:torvalds/linux.git", SSH: true}},
		{"gitlab.com/group/app", Repository{Host: "gitlab.com", Port: 22, Owner: "group", Name: "app", URL: "git@gitlab.com:group/app.git", SSH: true}},
		{"gitlab.com/group/sub/app", Repository{Host: "gitlab.com", Port: 22, Owner: "group/sub", Name: "app", URL: "git@gitlab.com:group/sub/app.git", SSH: true}},
		{"git@github.com:torvalds/linux.git", Repository{Host: "github.com", Port: 22, Owner: "torvalds", Name: "linux", URL: "git@github.com:torvalds/linux.git", SSH: true}},
		{"git@gitlab.com:group/sub/app.git", Repository{Host: "gitlab.com", Port: 22, Owner: "group/sub", Name: "app", URL: "git@gitlab.com:group/sub/app.git", SSH: true}},
		{"ssh://git@example.com:2222/team/app.git", Repository{Host: "example.com", Port: 2222, Owner: "team", Name: "app", URL: "ssh://git@example.com:2222/team/app.git", SSH: true}},
		{"https://github.com/torvalds/linux", Repository{Host: "github.com", Port: 443, Owner: "torvalds", Name: "linux", URL: "https://github.com/torvalds/linux"}},
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

// An argument that names no repository, or a folder outside the projects volume, is turned away as invalid.
func TestParseTurnsAwayWhatNamesNoRepository(t *testing.T) {
	// Try every malformed argument
	for _, arg := range []string{"linux", "", "a/b/c", "http://github.com/o/r", "git://github.com/o/r", "https://github.com/linux", "owner/..", "../linux", "github.com/../linux", "owner/a b", "git@..:o/r.git"} {
		_, err := Parse(arg)

		// Report an argument read as a repository
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) = %v, want %v", arg, err, ErrInvalid)
		}
	}
}

// A repository lands in the projects volume by host, owner and name.
func TestPathNestsByHostOwnerAndName(t *testing.T) {
	tests := map[string]string{
		"torvalds/linux":                    "/home/ubuntu/projects/github.com/torvalds/linux",
		"git@gitlab.com:group/sub/app.git":  "/home/ubuntu/projects/gitlab.com/group/sub/app",
		"https://example.com/team/app.git":  "/home/ubuntu/projects/example.com/team/app",
		"ssh://git@example.com:2222/team/x": "/home/ubuntu/projects/example.com/team/x",
	}
	// Place every repository
	for arg, want := range tests {
		r, err := Parse(arg)

		// Report a place that differs
		if err != nil || r.Path() != want {
			t.Errorf("Parse(%q).Path() = %q, %v, want %q", arg, r.Path(), err, want)
		}
	}
}

// A project's folder in the volume reads back as the repository it holds, in the short form Parse takes.
func TestAtReadsAFolderBack(t *testing.T) {
	r, ok := At("/home/ubuntu/projects/gitlab.com/group/sub/app")

	// Report a folder read wrong
	if !ok || r.Short() != "gitlab.com/group/sub/app" || r.FullName() != "group/sub/app" {
		t.Errorf("At = %+v, %v, want gitlab.com/group/sub/app", r, ok)
	}
	// Report a folder outside the volume read as a project
	if _, ok := At("/home/ubuntu/o3s"); ok {
		t.Errorf("At(/home/ubuntu/o3s) = true, want false")
	}
}
