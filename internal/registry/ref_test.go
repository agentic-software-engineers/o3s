package registry_test

import (
	"errors"
	"path"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/registry"
)

// A feature id splits into its store, its package and the tag it names.
func TestParseRefSplitsAFeatureID(t *testing.T) {
	tests := []struct {
		in   string
		want registry.Ref
	}{
		{fixture.Store + "/" + fixture.Feature, registry.Ref{Store: fixture.Store, ID: fixture.Feature}},
		{fixture.Store + "/" + fixture.Feature + ":" + fixture.Major, registry.Ref{Store: fixture.Store, ID: fixture.Feature, Tag: fixture.Major}},
		{"127.0.0.1:5000/team/features/" + fixture.Feature + ":" + fixture.MinorTag, registry.Ref{Store: "127.0.0.1:5000/team/features", ID: fixture.Feature, Tag: fixture.MinorTag}},
	}
	// Check every id
	for _, tt := range tests {
		got, err := registry.ParseRef(tt.in)

		// Report an id that splits otherwise
		if err != nil || got != tt.want {
			t.Errorf("ParseRef(%q) = %+v, %v, want %+v", tt.in, got, err, tt.want)
		}
	}
}

// A feature's package is its id stripped of the tag.
func TestRefPackageLeavesTheTagOut(t *testing.T) {
	r := registry.Ref{Store: fixture.Store, ID: fixture.Feature, Tag: fixture.Major}

	// Report a package that differs
	if got, want := r.Package(), fixture.Store+"/"+fixture.Feature; got != want {
		t.Errorf("Package() = %q, want %q", got, want)
	}
}

// An id that fails to name a feature in a store is turned away.
func TestParseRefTurnsAwayAMalformedID(t *testing.T) {
	// Check every malformed id
	for _, in := range []string{"", fixture.Feature, path.Dir(fixture.Store) + "/" + fixture.Feature, fixture.Store + "/Tool", fixture.Store + "/" + fixture.Feature + ":", fixture.Store + "/" + fixture.Feature + "@sha256:0"} {
		_, err := registry.ParseRef(in)

		// Require the error that marks a malformed id
		if !errors.Is(err, registry.ErrInvalid) {
			t.Errorf("ParseRef(%q) = %v, want ErrInvalid", in, err)
		}
	}
}
