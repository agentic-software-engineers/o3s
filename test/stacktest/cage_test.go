package stacktest

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// A run that names an archive hands it to every suite as it is.
func TestCageArchiveTakesTheRunsArchive(t *testing.T) {
	want := filepath.Join(t.TempDir(), "cage.tar")
	t.Setenv(archiveSetting, want)

	got := cageArchive(t)

	// Report an archive other than the run's
	if got != want {
		t.Errorf("cageArchive = %q, want %q", got, want)
	}
}

// A cage image counts as this commit's when its revision label names the commit the checkout holds.
func TestSameRevisionHoldsTheImageToTheCheckout(t *testing.T) {
	repo := Repo(t)
	head, err := revisionOf(repo)
	// Stop when the checkout's commit stays unnamed
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	readFailed := errors.New("no such image")
	tests := []struct {
		name  string
		label string
		read  error
		ok    bool
	}{
		{"this commit, as the inspect prints it", head + "\n", nil, true},
		{"another commit", fixture.Commit, nil, false},
		{"no label", "", nil, false},
		{"an image that fails to read", "", readFailed, false},
	}
	// Check every image
	for _, tt := range tests {
		err := sameRevision(tt.label, tt.read, repo)

		// Report an image held to the wrong verdict
		if (err == nil) != tt.ok {
			t.Errorf("%s: sameRevision = %v, want ok %t", tt.name, err, tt.ok)
		}
	}
}
