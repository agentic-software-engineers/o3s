// Package install_test runs install.sh against a release served from the test, on a host the test plays.
package install_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
)

// script is the installer under test.
const script = "../../install.sh"

// stub stands in for the o3s binary of a release.
const stub = "#!/bin/sh\necho o3s version " + fixture.Version + "\n"

// tools are the commands install.sh may use beyond the ones a test fakes.
var tools = []string{"sh", "curl", "sha256sum", "mktemp", "install", "rm", "mkdir", "grep", "cat", "ln", "head", "sed", "tar", "tr", "dirname"}

// feed is a releases feed whose newest entry is a release candidate.
const feed = `<feed>
<link href="https://github.com/hansehart/o3s/releases/tag/v` + fixture.Candidate + `"/>
<link href="https://github.com/hansehart/o3s/releases/tag/v` + fixture.Version + `"/>
</feed>
`

// aRelease serves a release holding the stub for Linux and macOS on amd64 and arm64, and checksums that match it unless tampered.
func aRelease(t *testing.T, tampered bool) string {
	t.Helper()
	dir := t.TempDir()
	sum := sha256.Sum256([]byte(stub))
	digest := hex.EncodeToString(sum[:])
	// Break the checksum where the test plays a tampered download
	if tampered {
		digest = strings.Repeat("0", 64)
	}
	files := map[string]string{"releases.atom": feed}
	sums := ""
	// Serve the stub for every system and architecture a release ships
	for _, asset := range []string{"o3s-linux-amd64", "o3s-linux-arm64", "o3s-darwin-amd64", "o3s-darwin-arm64"} {
		files[asset] = stub
		sums += digest + "  " + asset + "\n"
	}
	files["checksums.txt"] = sums + strings.Repeat("1", 64) + "  install.sh\n"
	// Write every file of the release
	for name, body := range files {
		// Stop when a file stays unwritten
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("Setup: %v", err)
		}
	}
	s := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(s.Close)
	return s.URL
}

// host is a machine the test plays, with the tools install.sh may use and the fakes it adds.
type host struct {
	home, bin string
	env       []string
}

// aHost builds a host that reports a kernel and an architecture, and runs the fake commands given.
func aHost(t *testing.T, kernel, arch string, fakes map[string]string) host {
	t.Helper()
	h := host{home: t.TempDir(), bin: t.TempDir()}
	// Link every tool the script may use from this machine
	for _, tool := range tools {
		path, err := exec.LookPath(tool)
		// Stop when this machine lacks a tool the script needs
		if err != nil {
			t.Fatalf("Setup: %s: %v", tool, err)
		}
		// Stop when the tool fails to link
		if err := os.Symlink(path, filepath.Join(h.bin, tool)); err != nil {
			t.Fatalf("Setup: %v", err)
		}
	}
	fakes["uname"] = "case \"$1\" in -s) echo " + kernel + " ;; -m) echo " + arch + " ;; esac\n"
	// Write every fake command
	for name, body := range fakes {
		// Stop when a fake stays unwritten
		if err := os.WriteFile(filepath.Join(h.bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatalf("Setup: %v", err)
		}
	}
	h.env = []string{"HOME=" + h.home, "PATH=" + h.bin}
	return h
}

// install runs install.sh on the host against a release, without the devcontainer CLI and Lima.
func (h host) install(t *testing.T, release string) (string, error) {
	t.Helper()
	return h.run(t, release, "--no-devcontainer", "--no-lima")
}

// run runs install.sh on the host against a release with the options given.
func (h host) run(t *testing.T, release string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "sh", append([]string{script}, args...)...)
	cmd.Env = append(slices.Clone(h.env), "O3S_RELEASE_URL="+release, "O3S_RELEASES_FEED="+release+"/releases.atom")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// installed reports whether the host holds the stable channel's binary in its install dir.
func (h host) installed() bool {
	return h.holds("o3s-latest")
}

// holds reports whether the host holds an executable binary of a name in its install dir.
func (h host) holds(name string) bool {
	info, err := os.Stat(filepath.Join(h.home, ".local", "bin", name))
	return err == nil && info.Mode()&0o111 != 0
}

// The binary lands executable in ~/.local/bin, with hints for PATH and for verifying it by hand.
func TestInstallPutsTheBinaryInPlace(t *testing.T) {
	h := aHost(t, "Linux", "x86_64", map[string]string{})

	out, err := h.install(t, aRelease(t, false))

	// Stop when the install fails
	if err != nil {
		t.Fatalf("install.sh = %v\n%s", err, out)
	}
	link, _ := os.Readlink(filepath.Join(h.home, ".local", "bin", "o3s"))
	// Require o3s-latest in place, and o3s a link to it
	if !h.installed() || link != "o3s-latest" {
		t.Errorf("install.sh left o3s-latest %v and o3s linking to %q, want both\n%s", h.installed(), link, out)
	}
	// Require every line a user needs next
	for _, want := range []string{"Installed o3s version " + fixture.Version, "export PATH=", "gh attestation verify", "--signer-workflow hansehart/o3s/.github/workflows/publish.yaml", "o3s init"} {
		// Report a line the output lacks
		if !strings.Contains(out, want) {
			t.Errorf("install.sh output lacks %q:\n%s", want, out)
		}
	}
}

// A download whose checksum differs stops the install before anything is written.
func TestInstallStopsOnATamperedDownload(t *testing.T) {
	h := aHost(t, "Linux", "x86_64", map[string]string{})

	out, err := h.install(t, aRelease(t, true))

	// Require the install to stop
	if err == nil || h.installed() {
		t.Errorf("install.sh on a tampered download = %v, installed %v, want a stop\n%s", err, h.installed(), out)
	}
	// Require the reason
	if !strings.Contains(out, "checksums.txt") {
		t.Errorf("install.sh output lacks the checksum reason:\n%s", out)
	}
}

// A host o3s ships no binary for is named and turned away.
func TestInstallTurnsAwayAnUnsupportedHost(t *testing.T) {
	tests := []struct{ kernel, arch, want string }{
		{"Linux", "riscv64", "riscv64"},
		{"FreeBSD", "amd64", "FreeBSD"},
	}
	// Check every host
	for _, tt := range tests {
		h := aHost(t, tt.kernel, tt.arch, map[string]string{})

		out, err := h.install(t, aRelease(t, false))

		// Require a stop that names the host
		if err == nil || !strings.Contains(out, tt.want) {
			t.Errorf("install.sh on %s %s = %v, want a stop naming %s\n%s", tt.kernel, tt.arch, err, tt.want, out)
		}
	}
}

// The checksum alone decides the install, the way other installers decide it, and the attestation stays a command the user runs, even where gh is logged in.
func TestInstallLeavesTheAttestationToTheUser(t *testing.T) {
	gh := "echo \"$*\" >> \"$HOME/gh.log\"\nexit 1\n"
	h := aHost(t, "Linux", "x86_64", map[string]string{"gh": gh})

	out, err := h.install(t, aRelease(t, false))

	// Require the install to pass on its checksum alone
	if err != nil || !h.installed() {
		t.Fatalf("install.sh = %v, installed %v, want it installed\n%s", err, h.installed(), out)
	}
	// Require the command that checks the attestation by hand
	if !strings.Contains(out, "gh attestation verify") {
		t.Errorf("install.sh output lacks the command to verify the attestation:\n%s", out)
	}
	// Require gh left alone
	if calls, err := os.ReadFile(filepath.Join(h.home, "gh.log")); err == nil {
		t.Errorf("install.sh ran gh: %s", calls)
	}
}

// A host without xz, which the devcontainer CLI's installer unpacks Node with, is told what to do before anything installs.
func TestInstallNamesTheMissingXz(t *testing.T) {
	h := aHost(t, "Linux", "x86_64", map[string]string{})

	out, err := h.run(t, aRelease(t, false), "--no-lima")

	// Require a stop that names xz and the way around it
	if err == nil || !strings.Contains(out, "xz") || !strings.Contains(out, "--no-devcontainer") {
		t.Errorf("install.sh without xz = %v, want a stop naming xz and --no-devcontainer\n%s", err, out)
	}
	// Require the host left as it was, so one run after installing xz does it all
	if h.installed() {
		t.Errorf("install.sh without xz installed o3s anyway")
	}
}

// The workflow install.sh accepts as the signer lives in the repo, so a rename fails here before it fails every install.
func TestTheSignerWorkflowExists(t *testing.T) {
	data, err := os.ReadFile(script)
	// Stop when the script fails to read
	if err != nil {
		t.Fatalf("ReadFile(install.sh) = %v", err)
	}
	m := regexp.MustCompile(`(?m)^signer="\$\{repo\}/(.+)"$`).FindSubmatch(data)
	// Stop when the script pins no signer
	if m == nil {
		t.Fatalf("install.sh pins no signer")
	}

	_, err = os.Stat(filepath.Join("../..", string(m[1])))

	// Report a signer the repo lacks
	if err != nil {
		t.Errorf("signer %s: %v", m[1], err)
	}
}

// A devcontainer CLI o3s installed before is kept, so a second install downloads nothing for it.
func TestInstallKeepsTheDevcontainerCLIItInstalled(t *testing.T) {
	h := aHost(t, "Linux", "x86_64", map[string]string{})
	bin := filepath.Join(h.home, ".local", "share", "o3s", "devcontainers", "bin")
	// Stop when the installed CLI stays unwritten
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the installed CLI stays unwritten
	if err := os.WriteFile(filepath.Join(bin, "devcontainer"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	out, err := h.run(t, aRelease(t, false), "--no-lima")

	// Require the install to keep the CLI, though the host lacks what a fresh install of it needs
	if err != nil || !strings.Contains(out, "Found the devcontainer CLI in "+filepath.Dir(bin)) {
		t.Errorf("install.sh with the CLI in place = %v, want it kept\n%s", err, out)
	}
}

// --rc installs the newest release, release candidates included, as o3s-rc, and --version a release as o3s-<version>, each beside o3s-latest.
func TestInstallNamesTheChannelsBinary(t *testing.T) {
	tests := []struct {
		args       []string
		binary     string
		mentioning string
	}{
		{[]string{"--rc"}, "o3s-rc", fixture.Candidate},
		{[]string{"--version", fixture.Version}, "o3s-" + fixture.Version, "o3s-" + fixture.Version + " init"},
	}
	// Install every channel on a fresh host
	for _, tt := range tests {
		h := aHost(t, "Linux", "x86_64", map[string]string{})

		out, err := h.run(t, aRelease(t, false), append(tt.args, "--no-devcontainer", "--no-lima")...)

		// Require the channel's binary in place, the stable one left alone
		if err != nil || !h.holds(tt.binary) || h.holds("o3s-latest") {
			t.Errorf("install.sh %v = %v, %s %v, o3s-latest %v\n%s", tt.args, err, tt.binary, h.holds(tt.binary), h.holds("o3s-latest"), out)
		}
		// Require the output to name what it took
		if !strings.Contains(out, tt.mentioning) {
			t.Errorf("install.sh %v output lacks %q:\n%s", tt.args, tt.mentioning, out)
		}
	}
}

// A Mac gets the binary built for it, under the same names as on Linux.
func TestInstallPutsTheMacBinaryInPlace(t *testing.T) {
	h := aHost(t, "Darwin", "arm64", map[string]string{})

	out, err := h.install(t, aRelease(t, false))

	// Require the stable channel's binary in place
	if err != nil || !h.installed() {
		t.Errorf("install.sh on Darwin arm64 = %v, installed %v, want o3s-latest\n%s", err, h.installed(), out)
	}
}

// aLima puts a Lima on the host where o3s installs its own, so the install finds it in place.
func aLima(t *testing.T, h host) string {
	t.Helper()
	bin := filepath.Join(h.home, ".local", "share", "o3s", "lima", "bin")
	// Stop when the folder of the Lima in place fails to appear
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the Lima in place stays unwritten
	if err := os.WriteFile(filepath.Join(bin, "limactl"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return filepath.Dir(bin)
}

// A Lima o3s installed before is kept, so a second install downloads nothing for it.
func TestInstallKeepsTheLimaItInstalled(t *testing.T) {
	h := aHost(t, "Darwin", "arm64", map[string]string{})
	prefix := aLima(t, h)

	out, err := h.run(t, aRelease(t, false), "--no-devcontainer", "--preview")

	// Require the install to keep Lima
	if err != nil || !strings.Contains(out, "Found Lima in "+prefix) {
		t.Errorf("install.sh with Lima in place = %v, want it kept\n%s", err, out)
	}
}

// A Lima archive that differs from its pinned checksum stays unpacked.
func TestInstallStopsOnATamperedLima(t *testing.T) {
	h := aHost(t, "Darwin", "arm64", map[string]string{})
	mirror := t.TempDir()
	// Stop when the tampered archive stays unwritten
	if err := os.WriteFile(filepath.Join(mirror, "lima-2.3.0-Darwin-arm64.tar.gz"), []byte("tampered"), 0o600); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	s := httptest.NewServer(http.FileServer(http.Dir(mirror)))
	t.Cleanup(s.Close)
	h.env = append(h.env, "O3S_LIMA_URL="+s.URL)

	out, err := h.run(t, aRelease(t, false), "--no-devcontainer", "--preview")

	_, unpacked := os.Stat(filepath.Join(h.home, ".local", "share", "o3s", "lima", "bin", "limactl"))
	// Require a stop that names the checksum, with Lima left out
	if err == nil || !strings.Contains(out, "Lima") || !strings.Contains(out, "pinned checksum") || unpacked == nil {
		t.Errorf("install.sh with a tampered Lima = %v, unpacked %v, want a stop\n%s", err, unpacked == nil, out)
	}
}

// A Linux host without QEMU, which Lima runs VMs with there, is told how to add it.
func TestInstallNamesTheMissingQEMU(t *testing.T) {
	h := aHost(t, "Linux", "x86_64", map[string]string{})
	aLima(t, h)

	out, err := h.run(t, aRelease(t, false), "--no-devcontainer", "--preview")

	// Require the install to finish and name QEMU's packages
	if err != nil || !strings.Contains(out, "QEMU") || !strings.Contains(out, "qemu-system-x86") {
		t.Errorf("install.sh without QEMU = %v, want a hint naming QEMU\n%s", err, out)
	}
}

// Without the VM preview, the install leaves Lima and QEMU alone, as the VM is off by default.
func TestInstallLeavesLimaToTheVMPreview(t *testing.T) {
	h := aHost(t, "Linux", "x86_64", map[string]string{})

	out, err := h.run(t, aRelease(t, false), "--no-devcontainer")

	_, unpacked := os.Stat(filepath.Join(h.home, ".local", "share", "o3s", "lima"))
	// Require o3s in place, with neither Lima nor a word about it or QEMU
	if err != nil || !h.installed() || unpacked == nil || strings.Contains(out, "Lima") || strings.Contains(out, "QEMU") {
		t.Errorf("install.sh without the preview = %v, Lima unpacked %v, want o3s alone\n%s", err, unpacked == nil, out)
	}
}

// O3S_PREVIEW turns the VM preview on for the install, as --preview does.
func TestInstallReadsTheVMPreviewFromTheEnvironment(t *testing.T) {
	h := aHost(t, "Linux", "x86_64", map[string]string{})
	aLima(t, h)
	h.env = append(h.env, "O3S_PREVIEW=vm")

	out, err := h.run(t, aRelease(t, false), "--no-devcontainer")

	// Require the install to take the Lima step
	if err != nil || !strings.Contains(out, "Found Lima in") {
		t.Errorf("install.sh with O3S_PREVIEW=vm = %v, want the Lima step\n%s", err, out)
	}
}
