package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/paths"
)

// A home's state folder names the home and holds a socket folder private to the user.
func TestHomeStateRecordsTheHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	home := "/home/ubuntu/o3s"

	err := HomeState(home)

	// Stop when the state stays unwritten
	if err != nil {
		t.Fatalf("HomeState() = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(paths.Home(home), "home.json"))
	// Stop when the record fails to read
	if err != nil {
		t.Fatalf("ReadFile(home.json) = %v", err)
	}
	var record struct{ Path string }
	// Require the record to name the home
	if err := json.Unmarshal(data, &record); err != nil || record.Path != home {
		t.Errorf("home.json = %s, %v, want the path %s", data, err, home)
	}
	info, err := os.Stat(paths.Home(home))
	// Require the home's folder for its owner alone
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("%s: %v, want a folder at 0700", paths.Home(home), err)
	}
	entries, _ := os.ReadDir(paths.Home(home))
	// Require the folder to hold the home's record alone
	if len(entries) != 1 || entries[0].Name() != "home.json" {
		t.Errorf("%s holds %v, want home.json alone", paths.Home(home), entries)
	}
}

// The values compose reads name the proxy's CA folder and the cage's socket folder.
func TestEnvNamesTheFoldersComposeMounts(t *testing.T) {
	state, home := t.TempDir(), t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("XDG_RUNTIME_DIR", "")
	// Stop when the stack folder fails to appear
	if err := os.MkdirAll(filepath.Join(home, ".devcontainer"), 0o750); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err := Env(home, "o3s-cage")

	// Stop when the values stay unwritten
	if err != nil {
		t.Fatalf("Env() = %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(home, ".devcontainer", ".env"))
	want := []string{
		"O3S_PROXY_DIR=" + paths.Proxy() + "\n",
		"O3S_SOCKETS=" + paths.CageSockets(home) + "\n",
	}
	// Require every folder named
	for _, line := range want {
		// Report a folder the file lacks
		if !strings.Contains(string(data), line) {
			t.Errorf(".env lacks %q:\n%s", line, data)
		}
	}
	// Require those two folders to be every folder the file names
	if got := len(regexp.MustCompile(`(?m)^O3S_[A-Z_]+=/`).FindAll(data, -1)); got != len(want) {
		t.Errorf(".env names %d folders, want %d:\n%s", got, len(want), data)
	}
}

// The homes this machine holds besides one are those whose folders still exist, the home itself left out.
func TestOtherHomesListsTheLiveOnes(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	this, other, gone := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "gone")
	// Record every home on this machine
	for _, home := range []string{this, other, gone} {
		// Stop when a home's state stays unwritten
		if err := HomeState(home); err != nil {
			t.Fatalf("Setup: HomeState(%s) = %v", home, err)
		}
	}

	got := OtherHomes(this)

	// Require the other live home alone
	if len(got) != 1 || got[0] != other {
		t.Errorf("OtherHomes() = %v, want [%s]", got, other)
	}
}
