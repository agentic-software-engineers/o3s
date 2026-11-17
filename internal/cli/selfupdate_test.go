package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// binaryAt stands in for the o3s binary of a release.
func binaryAt(version string) string {
	return "#!/bin/sh\necho o3s version " + version + "\n"
}

// aReleasePage serves releases: the stable one /latest names, and every version the feed lists, each with its binary and checksums.
func aReleasePage(t *testing.T, stable string, versions ...string) {
	t.Helper()
	asset := "o3s-linux-" + runtime.GOARCH
	mux := http.NewServeMux()
	mux.HandleFunc("GET /o3s/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/o3s/releases/tag/v"+stable, http.StatusFound)
	})
	var feed strings.Builder
	feed.WriteString("<feed>\n")
	all := append([]string{stable}, versions...)
	slices.Sort(all)
	// Serve every release once with its binary and checksums, and list it in the feed
	for _, v := range slices.Compact(all) {
		bin := binaryAt(v)
		sum := sha256.Sum256([]byte(bin))
		fmt.Fprintf(&feed, "<link href=\"https://github.com/hansehart/o3s/releases/tag/v%s\"/>\n", v)
		mux.HandleFunc("GET /o3s/releases/download/v"+v+"/"+asset, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(bin))
		})
		mux.HandleFunc("GET /o3s/releases/download/v"+v+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n"))
		})
	}
	feed.WriteString("</feed>\n")
	mux.HandleFunc("GET /o3s/releases.atom", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(feed.String()))
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	t.Setenv("O3S_RELEASES_URL", s.URL+"/o3s/releases")
}

// aRunningBinary plays the running binary at a release under a name, in a folder the test owns, and records the restart.
func aRunningBinary(t *testing.T, at, name string) (string, *[]string) {
	t.Helper()
	old := Version
	Version = at
	t.Cleanup(func() { Version = old })
	path := filepath.Join(t.TempDir(), name)
	// Stop when the running binary stays unwritten
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil { //nolint:gosec // the test plays a binary
		t.Fatalf("Setup: %v", err)
	}
	var restarted []string
	oldExecutable, oldRestart := executable, restartAs
	t.Cleanup(func() { executable, restartAs = oldExecutable, oldRestart })
	executable = func() (string, error) { return path, nil }
	restartAs = func(_ string, args []string) error { restarted = args; return nil }
	t.Setenv("PATH", t.TempDir())
	return path, &restarted
}

// contents reads a file, or names that it is missing.
func contents(path string) string {
	data, err := os.ReadFile(path)
	// Name a file that fails to read
	if err != nil {
		return "missing"
	}
	return string(data)
}

// Each channel updates within itself, replacing its own file and handing the rest of the update to it.
func TestSelfUpdateFollowsTheChannel(t *testing.T) {
	tests := []struct {
		name, binary, running, stable, want string
		feed                                []string
	}{
		{"latest moves to the newest stable", "o3s-latest", fixture.Version, fixture.NewerVersion, fixture.NewerVersion, []string{fixture.FurtherCandidate}},
		{"rc moves to the next rc", "o3s-rc", fixture.Candidate, fixture.Version, fixture.NextCandidate, []string{fixture.NextCandidate}},
		{"rc moves to the stable it leads to", "o3s-rc", fixture.NextCandidate, fixture.NewerVersion, fixture.NewerVersion, []string{fixture.NextCandidate}},
	}
	// Update every channel
	for _, tt := range tests {
		aReleasePage(t, tt.stable, tt.feed...)
		path, restarted := aRunningBinary(t, tt.running, tt.binary)
		var out strings.Builder

		done, err := selfUpdate(t.Context(), strings.NewReader(""), &out, &out, true)

		// Require the update done in place, under the channel's own name
		if err != nil || !done || contents(path) != binaryAt(tt.want) {
			t.Errorf("%s: selfUpdate() = %v, %v, binary %q, want %s\n%s", tt.name, done, err, contents(path), tt.want, out.String())
		}
		// Require the new binary to take over the update
		if want := []string{path, "update", "--yes"}; !slices.Equal(*restarted, want) {
			t.Errorf("%s: restarted as %q, want %q", tt.name, *restarted, want)
		}
	}
}

// A plain o3s from before the channels becomes o3s-latest, and o3s a link to it, so homes that name o3s keep working.
func TestSelfUpdateMovesAPlainBinaryToLatest(t *testing.T) {
	aReleasePage(t, fixture.NewerVersion)
	path, restarted := aRunningBinary(t, fixture.Version, "o3s")
	ghLog := aLoggedInGh(t)
	latestPath := filepath.Join(filepath.Dir(path), "o3s-latest")
	var out strings.Builder

	done, err := selfUpdate(t.Context(), strings.NewReader(""), &out, &out, true)

	// Stop when the update fails
	if err != nil || !done {
		t.Fatalf("selfUpdate() = %v, %v, want it done\n%s", done, err, out.String())
	}
	link, _ := os.Readlink(path)
	// Require o3s-latest in place and o3s linking to it
	if contents(latestPath) != binaryAt(fixture.NewerVersion) || link != "o3s-latest" {
		t.Errorf("o3s-latest = %q, o3s links to %q, want the release and o3s-latest", contents(latestPath), link)
	}
	// Require o3s-latest to take over the update
	if want := []string{latestPath, "update", "--yes"}; !slices.Equal(*restarted, want) {
		t.Errorf("restarted as %q, want %q", *restarted, want)
	}
	// Require the version line, the confirmation and the hint for checking by hand
	for _, want := range []string{fmt.Sprintf("o3s            %s -> %s", fixture.Version, fixture.NewerVersion), "Updated o3s to " + fixture.NewerVersion, "gh attestation verify " + latestPath} {
		// Report a line the output lacks
		if !strings.Contains(out.String(), want) {
			t.Errorf("selfUpdate() output lacks %q:\n%s", want, out.String())
		}
	}
	// Require gh left alone, since the checksum decides the update and the attestation stays the user's to check
	if calls, err := os.ReadFile(ghLog); err == nil {
		t.Errorf("selfUpdate() ran gh: %s", calls)
	}
}

// aLoggedInGh puts a gh on PATH that would answer as logged in and notes every call, so a test sees whether o3s runs it.
func aLoggedInGh(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "gh.log")
	gh := "#!/bin/sh\necho \"$*\" >> " + log + "\nexit 0\n"
	// Stop when the fake stays unwritten
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755); err != nil { //nolint:gosec // the test runs its fake
		t.Fatalf("Setup: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// A binary that has nothing to update to goes on to the home, with a note where the case gives one.
func TestSelfUpdateGoesOnToTheHome(t *testing.T) {
	tests := []struct {
		name, binary, version, note string
		reachable                   bool
	}{
		{"the latest release", "o3s-latest", fixture.Version, "", true},
		{"stable past a newer rc", "o3s-latest", fixture.Version, "", true},
		{"a pinned release", "o3s-" + fixture.Version, fixture.Version, fmt.Sprintf("o3s-%s stays at %s, install another with install.sh --version <X.Y.Z>", fixture.Version, fixture.Version), true},
		{"a dev build", "o3s", "dev", "o3s dev builds update with go install or install.sh", true},
		{"releases out of reach", "o3s-latest", fixture.Version, "warning: releases out of reach, o3s stays at " + fixture.Version, false},
	}
	// Check every case
	for _, tt := range tests {
		aReleasePage(t, fixture.Version, fixture.Candidate)
		// Point the releases at a closed port where the case plays them out of reach
		if !tt.reachable {
			t.Setenv("O3S_RELEASES_URL", "http://127.0.0.1:1")
		}
		path, restarted := aRunningBinary(t, tt.version, tt.binary)
		var out strings.Builder

		done, err := selfUpdate(t.Context(), strings.NewReader(""), &out, &out, true)

		// Require the update to go on with this binary, left as it was
		if err != nil || done || *restarted != nil || contents(path) != "old" {
			t.Errorf("%s: selfUpdate() = %v, %v, restarted %q, want to go on", tt.name, done, err, *restarted)
		}
		// Require the note the case gives, or none
		if strings.TrimSpace(out.String()) != tt.note {
			t.Errorf("%s: selfUpdate() output = %q, want %q", tt.name, out.String(), tt.note)
		}
	}
}
