package hostd_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/hostd"
	"github.com/hansehart/o3s/internal/paths"
)

// shortDir hands a test a short folder, since a socket's path has to fit the kernel's small limit.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "o3s") //nolint:usetesting // t.TempDir's long path outgrows a socket's path limit
	// Stop when the folder fails to appear
	if err != nil {
		t.Fatalf("Setup: MkdirTemp = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// With a runtime folder set, the sockets go into a folder of the home's own inside it, open to its owner alone.
func TestPlacePutsTheSocketsInTheRuntimeFolder(t *testing.T) {
	runtimeDir := shortDir(t)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("XDG_STATE_HOME", shortDir(t))

	p, err := hostd.PlaceOf(fixture.HomeDir)

	want := filepath.Join(runtimeDir, "o3s", paths.ID(fixture.HomeDir))
	// Stop when the place fails to settle
	if err != nil || p.Sockets != want {
		t.Fatalf("PlaceOf sockets = %q, %v, want %q", p.Sockets, err, want)
	}
	info, err := os.Stat(p.Sockets)
	// Require the folder to stand, open to its owner alone where the system knows modes
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Errorf("socket folder = %v, %v, want a folder with mode 0700", info, err)
	}
}

// While the runtime folder is unset, the sockets go into the home's own state folder.
func TestPlacePutsTheSocketsInTheStateFolderOtherwise(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_STATE_HOME", shortDir(t))

	p, err := hostd.PlaceOf(fixture.HomeDir)

	// Require the home's state folder
	if err != nil || p.Sockets != paths.Home(fixture.HomeDir) || p.State != paths.Home(fixture.HomeDir) {
		t.Errorf("PlaceOf = %+v, %v, want the sockets and the state in %q", p, err, paths.Home(fixture.HomeDir))
	}
}

// A folder whose socket paths outgrow the kernel's limit is turned away before anything listens.
func TestPlaceRefusesALongSocketPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(shortDir(t), strings.Repeat("d", 100)))
	t.Setenv("XDG_STATE_HOME", shortDir(t))

	_, err := hostd.PlaceOf(fixture.HomeDir)

	// Require the refusal to name the long path
	if !errors.Is(err, hostd.ErrPathTooLong) {
		t.Errorf("PlaceOf with a deep runtime folder = %v, want ErrPathTooLong", err)
	}
}

// The cage's sockets go into a folder of their own inside the sockets folder, apart from the control socket.
func TestPlaceKeepsTheCageSocketsApart(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", shortDir(t))
	t.Setenv("XDG_STATE_HOME", shortDir(t))

	p, err := hostd.PlaceOf(fixture.HomeDir)

	info, statErr := os.Stat(p.Cage)
	// Require the cage's folder inside the sockets folder, standing, with the control socket outside it
	if err != nil || statErr != nil || !info.IsDir() || filepath.Dir(p.Cage) != p.Sockets || filepath.Dir(p.Control()) != p.Sockets {
		t.Errorf("PlaceOf = %+v, %v, cage folder %v, want the cage folder inside %q and the control socket beside it", p, err, statErr, p.Sockets)
	}
}
