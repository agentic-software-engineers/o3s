package release_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/release"
)

// latest is the release the releases page offers as the newest.
const latest = fixture.NewerVersion

// binary stands in for the o3s binary of the latest release.
const binary = "#!/bin/sh\necho o3s version " + latest + "\n"

// asset is the name the release gives this host's binary.
var asset = "o3s-linux-" + runtime.GOARCH

// aRelease serves a releases page whose newest is the latest release, with checksums that match its binary unless tampered.
func aRelease(t *testing.T, tampered bool) {
	t.Helper()
	sum := sha256.Sum256([]byte(binary))
	digest := hex.EncodeToString(sum[:])
	// Break the checksum where the test plays a tampered download
	if tampered {
		digest = strings.Repeat("0", 64)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v"+latest, http.StatusFound)
	})
	mux.HandleFunc("GET /download/v"+latest+"/"+asset, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(binary))
	})
	mux.HandleFunc("GET /download/v"+latest+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(digest + "  " + asset + "\n" + strings.Repeat("1", 64) + "  install.sh\n"))
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	t.Setenv("O3S_RELEASES_URL", s.URL)
}

// The latest release is read from where the releases page redirects.
func TestLatestReadsTheRedirect(t *testing.T) {
	aRelease(t, false)

	got, err := release.Latest(t.Context())

	// Report a version that differs
	if err != nil || got != latest {
		t.Errorf("Latest() = %q, %v, want %s", got, err, latest)
	}
}

// A download that matches its checksum lands in the folder given.
func TestFetchKeepsAMatchingBinary(t *testing.T) {
	aRelease(t, false)

	path, err := release.Fetch(t.Context(), latest, t.TempDir())

	// Stop when the download fails
	if err != nil {
		t.Fatalf("Fetch() = %v", err)
	}
	data, err := os.ReadFile(path)
	// Require the release's binary
	if err != nil || string(data) != binary {
		t.Errorf("Fetch() wrote %q, %v, want the release's binary", data, err)
	}
}

// A download whose checksum differs is turned away.
func TestFetchTurnsAwayATamperedBinary(t *testing.T) {
	aRelease(t, true)

	_, err := release.Fetch(t.Context(), latest, t.TempDir())

	// Require the error that names a checksum mismatch
	if !errors.Is(err, release.ErrChecksum) {
		t.Errorf("Fetch() = %v, want ErrChecksum", err)
	}
}

// The hand check names the binary, this repo and the workflow that signs its releases.
func TestVerifyHintNamesThePublishWorkflow(t *testing.T) {
	got := release.VerifyHint("/tmp/o3s")

	want := "gh attestation verify /tmp/o3s --repo hansehart/o3s --signer-workflow hansehart/o3s/.github/workflows/publish.yaml"
	// Report a command that checks against something else
	if got != want {
		t.Errorf("VerifyHint() = %q, want %q", got, want)
	}
}

// Versions compare by semver precedence (semver.org, section 11): number by number, and a release candidate below its release.
func TestNewerComparesVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.99.99", true},
		{"1.10.0", "1.9.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
		{"1.0.0-rc.2", "1.0.0-rc.1", true},
		{"1.0.0-rc.10", "1.0.0-rc.2", true},
		{"1.0.0", "1.0.0-rc.1", true},
		{"1.0.0-rc.1", "0.9.9", true},
		{"1.0.0-rc.1", "1.0.0", false},
	}
	// Check every pair
	for _, tt := range tests {
		// Report an order that differs
		if got := release.Newer(tt.a, tt.b); got != tt.want {
			t.Errorf("Newer(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// Replacing puts the new binary at the old one's path, executable.
func TestReplaceSwapsTheBinary(t *testing.T) {
	dir := t.TempDir()
	old, src := filepath.Join(dir, "o3s"), filepath.Join(dir, "new")
	// Stop when either binary stays unwritten
	if err := os.WriteFile(old, []byte("old"), 0o755); err != nil { //nolint:gosec // the test plays a binary
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the new binary stays unwritten
	if err := os.WriteFile(src, []byte(binary), 0o600); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err := release.Replace(old, src)

	// Stop when the swap fails
	if err != nil {
		t.Fatalf("Replace() = %v", err)
	}
	data, _ := os.ReadFile(old)
	info, _ := os.Stat(old)
	// Require the new binary in place, executable
	if string(data) != binary || info.Mode().Perm() != 0o755 {
		t.Errorf("Replace() left %q with mode %v, want the new binary with 0755", data, info.Mode().Perm())
	}
}

// A release names three numbers, with or without a release candidate, and a build of no release names something else.
func TestIsReleaseKnowsReleaseCandidates(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{fixture.Version, true},
		{fixture.Candidate, true},
		{"dev", false},
		{fixture.DevBuild, false},
		{"1.0", false},
	}
	// Check every version
	for _, tt := range tests {
		// Report a version read otherwise
		if got := release.IsRelease(tt.version); got != tt.want {
			t.Errorf("IsRelease(%s) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
