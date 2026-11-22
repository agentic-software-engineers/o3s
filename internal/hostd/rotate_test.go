package hostd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/hostd"
)

// A log that outgrows its limit moves aside to one older file and starts afresh.
func TestRotatingKeepsOneOlderFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.log")
	line := fixture.Message + "\n"
	w, err := hostd.Rotating(path, int64(3*len(line)))
	// Stop when the log fails to open
	if err != nil {
		t.Fatalf("Setup: Rotating = %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	// Write more lines than three logs of the limit hold
	for range 7 {
		_, _ = w.Write([]byte(line))
	}

	current, _ := os.ReadFile(path)
	older, _ := os.ReadFile(path + ".1")
	_, thirdErr := os.Stat(path + ".2")
	// Require the newest line alone in the log, the three before it in the older file, and the older files at one
	if diff := cmp.Diff([]string{line, strings.Repeat(line, 3)}, []string{string(current), string(older)}); diff != "" || thirdErr == nil {
		t.Errorf("logs (-want +got):\n%s\nthird file present = %v", diff, thirdErr == nil)
	}
}
