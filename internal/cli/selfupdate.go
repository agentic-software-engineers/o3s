package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/release"
)

var (
	// executable finds the running binary's own file, which an update replaces.
	executable = func() (string, error) {
		path, err := os.Executable()
		// Hand back a binary whose file stays unknown
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(path)
	}
	// restartAs turns the running process into another binary, which carries the update on.
	restartAs = func(path string, args []string) error {
		return syscall.Exec(path, args, os.Environ())
	}
)

// selfUpdate brings the binary to the newest release of its channel and hands the parts of the update left to it, and
// reports whether it did. It ends with the binary where the rest is empty.
func selfUpdate(ctx context.Context, in io.Reader, out, errOut io.Writer, yes bool, rest []string) (bool, error) {
	current := version()
	// Leave a dev build to the way it was built
	if !release.IsRelease(current) {
		fmt.Fprintln(out, "o3s dev builds update with go install or install.sh")
		return false, nil
	}
	path, err := executable()
	// Hand back a binary whose file stays unknown
	if err != nil {
		return false, err
	}
	channel, pinned := release.ChannelOf(path)
	// Keep a pinned binary at its version, naming how to install another
	if channel == release.Pinned {
		fmt.Fprintf(out, "o3s-%s stays at %s, install another with install.sh --version <X.Y.Z>\n", pinned, pinned)
		return false, nil
	}
	target, err := newestOf(ctx, channel)
	// Go on with this binary where the releases stay out of reach
	if err != nil {
		warn(errOut, "releases out of reach, o3s stays at %s", current)
		return false, nil
	}
	// Go on with this binary where it is the newest of its channel
	if !release.Newer(target, current) {
		return false, nil
	}
	fmt.Fprintf(out, "%-14s %s -> %s\n", menu.CLI, current, target)
	ok, err := confirm(in, out, isTerminal(in), yes, "update")
	// Hand back an answer that fails to arrive, or go on with this binary when the user declines
	if err != nil || !ok {
		return false, err
	}
	dir, err := os.MkdirTemp("", "o3s-update-")
	// Hand back a download that lacks a folder
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(dir)
	bin, err := release.Fetch(ctx, target, dir)
	// Hand back a release that fails to download or to match its checksum
	if err != nil {
		return false, err
	}
	dest := path
	// Give a plain o3s from before the channels the name of the stable channel
	if filepath.Base(path) == "o3s" {
		dest = filepath.Join(filepath.Dir(path), "o3s-latest")
	}
	// Hand back a binary that stays in place
	if err := release.Replace(dest, bin); err != nil {
		return false, err
	}
	// Turn the plain o3s into a link to o3s-latest, so homes that name it keep working
	if dest != path {
		// Hand back a link that stays unmade
		if err := relink(path, filepath.Base(dest)); err != nil {
			return false, err
		}
	}
	fmt.Fprintf(out, "Updated o3s to %s\n", target)
	fmt.Fprintf(out, "Verify its build attestation with: %s\n", release.VerifyHint(dest))
	_ = os.RemoveAll(dir)
	// End where the binary was all the update named
	if rest == nil {
		return true, nil
	}
	args := append([]string{dest, "update"}, rest...)
	// Pass the answer on to the new binary's own question
	if yes {
		args = append(args, "--yes")
	}
	return true, restartAs(dest, args)
}

// newerRelease reads the release the binary's channel would update to and the binary's own, and whether that release
// is newer.
func newerRelease(ctx context.Context) (string, string, bool) {
	current := version()
	path, err := executable()
	// Report the binary current for a dev build or a binary whose file stays unknown
	if !release.IsRelease(current) || err != nil {
		return "", current, false
	}
	channel, _ := release.ChannelOf(path)
	// Report the binary current for a binary pinned to its version
	if channel == release.Pinned {
		return "", current, false
	}
	target, err := newestOf(ctx, channel)
	// Report the binary current where the releases stay out of reach
	if err != nil {
		return "", current, false
	}
	return target, current, release.Newer(target, current)
}

// newestOf reads the release a channel updates to: the latest stable release, or the newest release candidates included.
func newestOf(ctx context.Context, channel release.Channel) (string, error) {
	// Read the release candidates' feed for o3s-rc
	if channel == release.Candidates {
		return release.Newest(ctx)
	}
	return release.Latest(ctx)
}

// relink replaces a file with a link to a name beside it, in one step.
func relink(path, target string) error {
	tmp := path + ".link"
	_ = os.Remove(tmp)
	// Hand back a link that fails to appear
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
