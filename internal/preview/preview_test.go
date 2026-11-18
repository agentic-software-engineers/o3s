package preview_test

import (
	"testing"

	"github.com/hansehart/o3s/internal/preview"
)

// A preview is on where O3S_PREVIEW reads 1 or lists it among other names, and off otherwise.
func TestEnabledReadsTheEnvironment(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"1", true},
		{"vm", true},
		{"other, vm", true},
		{"other", false},
	}
	// Check every value the environment may hold
	for _, tt := range tests {
		t.Setenv("O3S_PREVIEW", tt.value)

		got := preview.Enabled(preview.VM)

		// Report a value read the wrong way
		if got != tt.want {
			t.Errorf("Enabled(vm) with O3S_PREVIEW=%q = %v, want %v", tt.value, got, tt.want)
		}
	}
}
