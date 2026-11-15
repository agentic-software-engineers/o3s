package stacktest

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Home hands back the folder of the test's home, its config joining the dev overlay.
func Home(tb testing.TB) string {
	tb.Helper()
	return devHome(tb)
}

// O3s runs o3s from this branch in the test's home, the way a user runs it there, and hands back what it printed on
// stdout and stderr together.
func O3s(tb testing.TB, args ...string) (string, error) {
	tb.Helper()
	dir := devHome(tb)
	bin, err := exec.LookPath("o3s")
	// Stop when the binary this branch built is off PATH
	if err != nil {
		tb.Fatalf("Setup: find o3s: %v", err)
	}
	cmd := exec.CommandContext(tb.Context(), bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	// Hand back the command's own output along with the failure
	if err != nil {
		return string(out), fmt.Errorf("o3s %s in %s: %w\n%s", strings.Join(args, " "), filepath.Base(dir), err, out)
	}
	return string(out), nil
}
