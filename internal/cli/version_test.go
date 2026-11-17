package cli

import (
	"runtime/debug"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// The version the release sets wins, a module build reads its own, and a checkout shows dev.
func TestVersionNamesTheBuild(t *testing.T) {
	module := &debug.BuildInfo{Main: debug.Module{Version: "v" + fixture.Version}}
	checkout := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}
	tests := []struct {
		name     string
		released string
		info     *debug.BuildInfo
		want     string
	}{
		{"a release", fixture.Version, checkout, fixture.Version},
		{"go install at a tag", "", module, fixture.Version},
		{"a checkout", "", checkout, "dev"},
		{"a build without its info", "", nil, "dev"},
	}
	// Check every kind of build
	for _, tt := range tests {
		// Report a version that differs
		if got := versionOf(tt.released, tt.info); got != tt.want {
			t.Errorf("%s: versionOf() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
