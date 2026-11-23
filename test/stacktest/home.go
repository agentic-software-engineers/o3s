package stacktest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// homes holds, per test, the o3s home made for it.
var homes sync.Map

// aHome makes an o3s home the way a user does, with o3s built from this branch, and hands back its folder.
//
// The test's XDG folders point at a folder of its own and the binary sits first on its PATH, so every command
// the test starts, the stack's start hook included, works in this home alone.
func aHome(tb testing.TB) string {
	tb.Helper()
	// Hand back the home this test already holds, so its stack keeps one identity
	if dir, ok := homes.Load(tb); ok {
		return dir.(string)
	}
	bin := o3sBinary(tb)
	tb.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	xdg := tb.TempDir()
	// Give every XDG folder a place of its own, below a folder the home leaves out
	for _, kind := range []string{"CONFIG", "STATE", "DATA"} {
		tb.Setenv("XDG_"+kind+"_HOME", filepath.Join(xdg, kind))
	}
	runtimeDir, err := os.MkdirTemp("", "o3s") //nolint:usetesting // t.TempDir's long path outgrows a socket's path limit
	// Stop when the runtime folder fails to appear
	if err != nil {
		tb.Fatalf("Setup: MkdirTemp = %v", err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	// Keep the host process's sockets in a short folder of the test's own
	tb.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	tb.Setenv("O3S_IMAGES", filepath.Join(Repo(tb), "images"))
	dir := filepath.Join(tb.TempDir(), "home")
	// Stop when the home's folder fails to appear, open to read as a folder a user makes with mkdir
	if err := os.Mkdir(dir, 0o755); err != nil { //nolint:gosec // the cage's user reads the home through its mount
		tb.Fatalf("Setup: %v", err)
	}
	cmd := exec.CommandContext(tb.Context(), bin, "init", "--dangerously-skip-vm")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	// Stop when the home stays unmade
	if err != nil {
		tb.Fatalf("Setup: o3s init: %v\n%s", err, out)
	}
	homes.Store(tb, dir)
	tb.Cleanup(func() { homes.Delete(tb) })
	tb.Cleanup(func() {
		stop := exec.CommandContext(context.WithoutCancel(tb.Context()), bin, "host", "stop")
		stop.Dir = dir
		_ = stop.Run()
	})
	return dir
}

// o3sBinary builds o3s from this branch for a test, so the home and its start hook run the code under review.
func o3sBinary(tb testing.TB) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "o3s")
	// Name the binary the way Windows finds a program
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	cmd := exec.CommandContext(tb.Context(), "go", "build", "-o", path, "./cmd/o3s")
	cmd.Dir = Repo(tb)
	out, err := cmd.CombinedOutput()
	// Stop when the binary fails to build
	if err != nil {
		tb.Fatalf("Setup: build o3s: %v\n%s", err, out)
	}
	return path
}
