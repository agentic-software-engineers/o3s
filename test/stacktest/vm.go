package stacktest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// AVMHost makes a host for a home's VM, with o3s built from this branch first on PATH, and hands back the home's empty folder.
//
// The XDG folders sit below a short temporary folder rather than the test's own, since Lima keeps sockets in the VM's folder below
// the data folder and a socket path is capped at about 104 characters (lima pkg/limayaml/validate.go:418).
func AVMHost(tb testing.TB) string {
	tb.Helper()
	bin := o3sBinary(tb)
	tb.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	xdg, err := os.MkdirTemp("", "o3s") //nolint:usetesting // tb.TempDir() is named after the test, too long for Lima's sockets
	// Stop when the XDG folders have no place to go
	if err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(xdg) })
	// Give every XDG folder a place of its own
	for _, kind := range []string{"CONFIG", "STATE", "DATA"} {
		tb.Setenv("XDG_"+kind+"_HOME", filepath.Join(xdg, kind))
	}
	dir := filepath.Join(xdg, "home")
	// Stop when the home's folder fails to appear
	if err := os.Mkdir(dir, 0o700); err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	return dir
}

// O3sIn runs the o3s on PATH in a home's folder, and hands back what it printed on stdout and stderr together.
func O3sIn(tb testing.TB, dir string, args ...string) (string, error) {
	tb.Helper()
	cmd := exec.CommandContext(tb.Context(), "o3s", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
