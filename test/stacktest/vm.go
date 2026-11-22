package stacktest

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// AVMHost makes a bare host for a home's VM and installs o3s on it the way a user does, with install.sh from release files
// built from this branch, and hands back the home's empty folder. The installer brings Lima along. Windows takes o3s built from
// this branch and the Lima on PATH until its installer joins.
//
// The folders sit below a short temporary folder, since the VM manager keeps sockets there and socket paths are capped in length.
func AVMHost(tb testing.TB) string {
	tb.Helper()
	base := ""
	// Keep the folder short on hosts with Unix sockets
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "o3s") //nolint:usetesting // tb.TempDir() is named after the test, too long for Lima's sockets
	// Stop when the host's folders fail to find a place
	if err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(root) })
	// Give every XDG folder a place of its own
	for _, kind := range []string{"CONFIG", "STATE", "DATA"} {
		tb.Setenv("XDG_"+kind+"_HOME", filepath.Join(root, kind))
	}
	release := aRelease(tb)
	tb.Setenv("O3S_RELEASE_URL", fileURL(release))
	bin := filepath.Join(root, "bin")
	// Install o3s with its installer, where the host has one
	if runtime.GOOS != "windows" {
		install := exec.CommandContext(tb.Context(), "sh", filepath.Join(release, "install.sh"), "--no-devcontainer")
		install.Env = append(os.Environ(), "O3S_INSTALL_DIR="+bin, "GH_TOKEN=", "GITHUB_TOKEN=", "GH_CONFIG_DIR="+tb.TempDir())
		out, err := install.CombinedOutput()
		// Stop when the install fails
		if err != nil {
			tb.Fatalf("Setup: install.sh: %v\n%s", err, out)
		}
		tb.Logf("install.sh:\n%s", out)
	} else {
		bin = filepath.Dir(o3sBinary(tb))
	}
	tb.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := filepath.Join(root, "home")
	// Stop when the home's folder fails to appear
	if err := os.Mkdir(dir, 0o700); err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	return dir
}

// aRelease lays out the release files a host installs from, o3s built from this branch for this host and for Linux, the system
// its VM runs, the way a bare machine's install reads a local build, and hands back their folder.
func aRelease(tb testing.TB) string {
	tb.Helper()
	dir := tb.TempDir()
	assets := []string{"o3s-linux-" + runtime.GOARCH}
	// Build for this host too, where it runs another system than its VM
	if runtime.GOOS != "linux" {
		assets = append(assets, "o3s-"+runtime.GOOS+"-"+runtime.GOARCH)
	}
	// Build every binary the release holds
	for _, asset := range assets {
		goos := strings.Split(asset, "-")[1]
		build := exec.CommandContext(tb.Context(), "go", "build", "-o", filepath.Join(dir, asset), "./cmd/o3s")
		build.Dir = Repo(tb)
		build.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
		// Stop when a build fails
		if out, err := build.CombinedOutput(); err != nil {
			tb.Fatalf("Setup: build %s: %v\n%s", asset, err, out)
		}
	}
	installer, err := os.ReadFile(filepath.Join(Repo(tb), "install.sh"))
	// Stop when the installer fails to read
	if err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	// Stop when the installer stays out of the release
	if err := os.WriteFile(filepath.Join(dir, "install.sh"), installer, 0o600); err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	var sums strings.Builder
	// Name every file's checksum, the way a release lists them
	for _, name := range append(assets, "install.sh") {
		data, err := os.ReadFile(filepath.Join(dir, name))
		// Stop when a release file fails to read
		if err != nil {
			tb.Fatalf("Setup: %v", err)
		}
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(data), name)
	}
	// Stop when the checksums stay unwritten
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(sums.String()), 0o600); err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	return dir
}

// fileURL names a local folder as a file URL, with the leading slash a Windows drive letter needs.
func fileURL(dir string) string {
	path := filepath.ToSlash(dir)
	// Lead a Windows drive letter with the slash a URL's path starts with
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + path
}

// O3sIn runs the o3s on PATH in a home's folder, and hands back what it printed on stdout, with stderr in its failure. Its stderr
// lands in the test's log either way, so a run that passes still shows what the VM manager did and when.
func O3sIn(tb testing.TB, dir string, args ...string) (string, error) {
	tb.Helper()
	cmd := exec.CommandContext(tb.Context(), "o3s", args...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	// Log what o3s printed on stderr, where it printed anything
	if stderr.Len() > 0 {
		tb.Logf("o3s %s stderr:\n%s", strings.Join(args, " "), stderr.String())
	}
	// Hand back the failure with what o3s printed on stderr
	if err != nil {
		return stdout.String(), fmt.Errorf("o3s %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}
