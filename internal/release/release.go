// Package release finds, downloads and checks the o3s releases a binary updates itself to.
package release

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/hansehart/o3s/internal/atomicfile"
)

const (
	// repo is where o3s is released.
	repo = "hansehart/o3s"
	// signer is the workflow that signs every release's build attestation.
	signer = repo + "/.github/workflows/publish.yaml"
)

// ErrChecksum marks a download that differs from its line in the release's checksums.
var ErrChecksum = errors.New("differs from the release's checksums")

// base is where the releases live, the environment's choice for a mirror or a test.
func base() string {
	// Take the address the environment names
	if b := os.Getenv("O3S_RELEASES_URL"); b != "" {
		return strings.TrimSuffix(b, "/")
	}
	return "https://github.com/" + repo + "/releases"
}

// Asset is the name a release gives the binary for this host.
func Asset() string {
	return "o3s-linux-" + runtime.GOARCH
}

// Latest reads the newest release's version from where the releases page redirects.
func Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base()+"/latest", nil)
	// Hand back a request that fails to build
	if err != nil {
		return "", err
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	// Hand back releases out of reach
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	tag := path.Base(resp.Header.Get("Location"))
	// Turn away an answer that names no release
	if resp.StatusCode/100 != 3 || !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("the releases page names no latest release (%s)", resp.Status)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

// Fetch downloads a release's binary for this host into a folder and checks it against the release's checksums.
func Fetch(ctx context.Context, version, dir string) (string, error) {
	url := base() + "/download/v" + version + "/"
	data, err := get(ctx, url+Asset())
	// Hand back a binary that fails to download
	if err != nil {
		return "", err
	}
	sums, err := get(ctx, url+"checksums.txt")
	// Hand back checksums that fail to download
	if err != nil {
		return "", err
	}
	want, ok := checksum(sums, Asset())
	// Turn away a release whose checksums lack the binary
	if !ok {
		return "", fmt.Errorf("checksums.txt of %s names no %s", version, Asset())
	}
	sum := sha256.Sum256(data)
	// Turn away a binary whose checksum differs
	if hex.EncodeToString(sum[:]) != want {
		return "", fmt.Errorf("%s of %s %w", Asset(), version, ErrChecksum)
	}
	target := filepath.Join(dir, Asset())
	return target, atomicfile.Write(target, data, 0o755)
}

// ErrNoFolder marks a build that has no release to take its files from.
var ErrNoFolder = errors.New("a dev build installs o3s into its VM from the folder O3S_RELEASE_URL names, holding o3s-linux-<arch>, install.sh and checksums.txt")

// Folder is where a version's release files live: the folder O3S_RELEASE_URL names, else the release's own downloads.
func Folder(version string) (string, error) {
	// Take the folder the environment names, the one install.sh reads too
	if folder := os.Getenv("O3S_RELEASE_URL"); folder != "" {
		return strings.TrimSuffix(folder, "/"), nil
	}
	// Turn away a build that names no release
	if !IsRelease(version) {
		return "", ErrNoFolder
	}
	return base() + "/download/v" + version, nil
}

// Bundle copies what installs o3s on a Linux machine of this host's architecture, its binary and install.sh, from a release
// folder into dir, with the checksums both are checked against.
func Bundle(ctx context.Context, folder, dir string) error {
	sums, err := get(ctx, folder+"/checksums.txt")
	// Hand back checksums that fail to download
	if err != nil {
		return err
	}
	// Copy and check every file the install needs
	for _, name := range []string{Asset(), "install.sh"} {
		data, err := get(ctx, folder+"/"+name)
		// Hand back a file that fails to download
		if err != nil {
			return err
		}
		want, ok := checksum(sums, name)
		sum := sha256.Sum256(data)
		// Turn away a file the checksums lack or that differs from them
		if !ok || hex.EncodeToString(sum[:]) != want {
			return fmt.Errorf("%s %w", name, ErrChecksum)
		}
		// Hand back a file that stays unwritten
		if err := atomicfile.Write(filepath.Join(dir, name), data, 0o755); err != nil {
			return err
		}
	}
	return atomicfile.Write(filepath.Join(dir, "checksums.txt"), sums, 0o644)
}

// Verify checks a binary's build attestation where gh is installed and logged in, and reports whether it checked.
func Verify(ctx context.Context, binary string) (bool, error) {
	gh, err := exec.LookPath("gh")
	// Leave the check to the user where gh is missing
	if err != nil {
		return false, nil
	}
	// Leave the check to the user where gh is logged out
	if exec.CommandContext(ctx, gh, "auth", "status").Run() != nil {
		return false, nil
	}
	out, err := exec.CommandContext(ctx, gh, "attestation", "verify", binary, "--repo", repo, "--signer-workflow", signer).CombinedOutput()
	// Turn away a binary whose attestation fails to verify
	if err != nil {
		return false, fmt.Errorf("the build attestation of %s fails to verify: %s", binary, strings.TrimSpace(string(out)))
	}
	return true, nil
}

// VerifyHint is the command a user runs to check a binary's build attestation by hand.
func VerifyHint(binary string) string {
	return "gh attestation verify " + binary + " --repo " + repo + " --signer-workflow " + signer
}

// Newer reports whether version a comes after version b by semver, a release candidate below the release it leads to.
func Newer(a, b string) bool {
	// Order nothing that reads as no release
	if !IsRelease(a) || !IsRelease(b) {
		return false
	}
	return semver.Compare("v"+a, "v"+b) > 0
}

// get downloads a URL whole, reading a file URL from the local disk.
func get(ctx context.Context, url string) ([]byte, error) {
	// Read a local file the way install.sh's curl reads a file URL
	if local, ok := strings.CutPrefix(url, "file://"); ok {
		return os.ReadFile(filepath.FromSlash(strings.TrimPrefix(local, driveSlash(local))))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	// Hand back a request that fails to build
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	// Hand back a download out of reach
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Turn away a download the server lacks
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// driveSlash is the slash a file URL puts before a Windows drive letter, which the local path leaves out.
func driveSlash(path string) string {
	// Name the slash before a drive letter such as /C:/
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		return "/"
	}
	return ""
}

// checksum finds a file's line in a checksums list.
func checksum(list []byte, name string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(string(list)))
	// Look at every line
	for sc.Scan() {
		sum, file, ok := strings.Cut(sc.Text(), "  ")
		// Take the line that names the file
		if ok && file == name {
			return sum, true
		}
	}
	return "", false
}

// candidate matches the pre-release part of a release candidate.
var candidate = regexp.MustCompile(`^-rc\.[0-9]+$`)

// IsRelease reports whether a version names a published release: three numbers, and a release candidate's -rc.N where it is one.
func IsRelease(version string) bool {
	v := "v" + version
	pre := semver.Prerelease(v)
	return semver.Canonical(v) == v && (pre == "" || candidate.MatchString(pre))
}
