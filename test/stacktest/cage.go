package stacktest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// cageImage is the name a home's stack runs its cage under, outside a release.
const cageImage = "ghcr.io/hansehart/o3s-cage:latest"

// revisionLabel is the OCI label the cage image carries the commit it was built from in.
const revisionLabel = "org.opencontainers.image.revision"

// archiveSetting names a cage image archive built from this commit, which a CI run builds once for every suite.
const archiveSetting = "O3S_CAGE_ARCHIVE"

// archived holds the archive a local run builds once for every test in the binary.
var archived = sync.OnceValues(buildArchive)

// loaded holds the host's load of the archive, once for every test in the binary.
var loaded sync.Once

// cageArchive hands back the archive of the cage image built from this commit: the one the run names, else one built here.
func cageArchive(tb testing.TB) string {
	tb.Helper()
	// Take the archive the run built for every suite
	if path := os.Getenv(archiveSetting); path != "" {
		return path
	}
	path, err := archived()
	// Stop when the cage image fails to build
	if err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	return path
}

// buildArchive builds the cage image from this checkout under the name a home's stack runs, and saves it as an archive.
func buildArchive() (string, error) {
	repo, err := repoRoot()
	// Hand back a checkout that fails to show its root
	if err != nil {
		return "", err
	}
	revision, err := revisionOf(repo)
	// Hand back a commit that stays unnamed
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "o3s-cage")
	// Hand back an archive that finds no place
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "cage.tar")
	steps := [][]string{
		{"build", "-q", "-t", cageImage, "--label", revisionLabel + "=" + revision, "-f", "images/cage/Dockerfile", "."},
		{"save", "-o", path, cageImage},
	}
	// Build the image, then save it
	for _, args := range steps {
		cmd := exec.Command("docker", args...) //nolint:noctx // the build outlives any one test that asks first
		cmd.Dir = repo
		// Hand back a step that fails, with what docker printed
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("docker %s: %w\n%s", args[0], err, out)
		}
	}
	return path, nil
}

// ACage loads the cage image built from this commit into the host's engine, so every stack the test raises runs it.
func ACage(tb testing.TB) {
	tb.Helper()
	archive := cageArchive(tb)
	loaded.Do(func() {
		// Stop when the archive fails to load
		if out, err := run(tb.Context(), tb, "docker", "load", "-q", "-i", archive); err != nil {
			tb.Fatalf("Setup: %v\n%s", err, out)
		}
	})
	out, err := run(tb.Context(), tb, "docker", "image", "inspect", "--format", labelFormat, cageImage)
	// Stop when the host's cage image comes from another commit
	if err := sameRevision(out, err, Repo(tb)); err != nil {
		tb.Fatalf("Setup: the host's cage image: %v", err)
	}
}

// ACageIn loads the cage image built from this commit into the engine of the home's VM, so the VM raises it.
func ACageIn(tb testing.TB, dir string) {
	tb.Helper()
	archive, err := os.Open(cageArchive(tb))
	// Stop when the archive fails to open
	if err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	defer func() { _ = archive.Close() }()
	load := exec.CommandContext(tb.Context(), "o3s", "cage", "shell", "--vm", "--", "docker", "load", "-q")
	load.Dir = dir
	load.Stdin = archive
	// Stop when the archive fails to load in the VM
	if out, err := load.CombinedOutput(); err != nil {
		tb.Fatalf("Setup: load the cage image in the VM: %v\n%s", err, out)
	}
	out, err := O3sIn(tb, dir, "cage", "shell", "--vm", "--", "docker", "image", "inspect", "--format", labelFormat, cageImage)
	// Stop when the VM's cage image comes from another commit
	if err := sameRevision(out, err, Repo(tb)); err != nil {
		tb.Fatalf("Setup: the VM's cage image: %v", err)
	}
}

// labelFormat reads the commit a cage image was built from.
const labelFormat = `{{index .Config.Labels "` + revisionLabel + `"}}`

// sameRevision requires an image's revision, as read by labelFormat, to name the commit the checkout holds.
func sameRevision(label string, read error, repo string) error {
	// Hand back an image that fails to show its labels
	if read != nil {
		return read
	}
	want, err := revisionOf(repo)
	// Hand back a commit that stays unnamed
	if err != nil {
		return err
	}
	// Hand back an image from another commit
	if got := strings.TrimSpace(label); got != want {
		return fmt.Errorf("built from %q, want %q", got, want)
	}
	return nil
}

// revisionOf names the commit a checkout holds.
func revisionOf(repo string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD") //nolint:noctx // a local read that ends at once
	cmd.Dir = repo
	out, err := cmd.Output()
	// Hand back a checkout git fails to read
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
