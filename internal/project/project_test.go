package project

import (
	"errors"
	"path"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
)

// The parts of the repositories the set names, each as the parser reads it.
var (
	owner, name, _            = strings.Cut(fixture.Repo, "/")
	nestedHost, nestedPath, _ = strings.Cut(fixture.NestedRepo, "/")
	nestedOwner               = path.Dir(nestedPath)
	nestedName                = path.Base(nestedPath)
)

// Every form a repository takes reads as the repository it names, the short ones cloned over SSH.
func TestParseReadsEveryForm(t *testing.T) {
	onHost := fixture.Host + "/" + fixture.Repo
	tests := []struct {
		arg  string
		want Repository
	}{
		{fixture.Repo, Repository{Host: "github.com", Port: 22, Owner: owner, Name: name, URL: "git@github.com:" + fixture.Repo + ".git", SSH: true}},
		{fixture.Repo + ".git", Repository{Host: "github.com", Port: 22, Owner: owner, Name: name, URL: "git@github.com:" + fixture.Repo + ".git", SSH: true}},
		{onHost, Repository{Host: fixture.Host, Port: 22, Owner: owner, Name: name, URL: "git@" + fixture.Host + ":" + fixture.Repo + ".git", SSH: true}},
		{fixture.NestedRepo, Repository{Host: nestedHost, Port: 22, Owner: nestedOwner, Name: nestedName, URL: "git@" + nestedHost + ":" + nestedPath + ".git", SSH: true}},
		{"git@github.com:" + fixture.Repo + ".git", Repository{Host: "github.com", Port: 22, Owner: owner, Name: name, URL: "git@github.com:" + fixture.Repo + ".git", SSH: true}},
		{"git@" + nestedHost + ":" + nestedPath + ".git", Repository{Host: nestedHost, Port: 22, Owner: nestedOwner, Name: nestedName, URL: "git@" + nestedHost + ":" + nestedPath + ".git", SSH: true}},
		{"ssh://git@" + fixture.Host + ":2222/" + fixture.Repo + ".git", Repository{Host: fixture.Host, Port: 2222, Owner: owner, Name: name, URL: "ssh://git@" + fixture.Host + ":2222/" + fixture.Repo + ".git", SSH: true}},
		{"https://" + onHost, Repository{Host: fixture.Host, Port: 443, Owner: owner, Name: name, URL: "https://" + onHost}},
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
		fixture.Repo: Dir + "/github.com/" + fixture.Repo,
		"git@" + nestedHost + ":" + nestedPath + ".git":         Dir + "/" + fixture.NestedRepo,
		"https://" + fixture.Host + "/" + fixture.Repo + ".git": Dir + "/" + fixture.Host + "/" + fixture.Repo,
		"ssh://git@" + fixture.Host + ":2222/" + fixture.Repo:   Dir + "/" + fixture.Host + "/" + fixture.Repo,
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
	r, ok := At(Dir + "/" + fixture.NestedRepo)

	// Report a folder read wrong
	if !ok || r.Short() != fixture.NestedRepo || r.FullName() != nestedPath {
		t.Errorf("At = %+v, %v, want %s", r, ok, fixture.NestedRepo)
	}
	// Report a folder outside the volume read as a project
	if _, ok := At("/home/ubuntu/o3s"); ok {
		t.Errorf("At(/home/ubuntu/o3s) = true, want false")
	}
}
