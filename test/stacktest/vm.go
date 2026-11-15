package stacktest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// AVMHost makes a host for a home's VM, with o3s built from this branch first on PATH, and hands back the home's empty folder.
//
// The XDG folders sit below a short temporary folder, since the VM manager keeps sockets there and socket paths are capped in length.
func AVMHost(tb testing.TB) string {
	tb.Helper()
	bin := o3sBinary(tb)
	tb.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	base := ""
	// Keep the folder short on hosts with Unix sockets
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	xdg, err := os.MkdirTemp(base, "o3s") //nolint:usetesting // tb.TempDir() is named after the test, too long for Lima's sockets
	// Stop when the XDG folders' place fails to appear
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

// O3sIn runs the o3s on PATH in a home's folder, and hands back what it printed on stdout, with stderr in its failure.
func O3sIn(tb testing.TB, dir string, args ...string) (string, error) {
	tb.Helper()
	cmd := exec.CommandContext(tb.Context(), "o3s", args...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// Hand back what o3s printed on stderr along with the failure
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("o3s %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}
