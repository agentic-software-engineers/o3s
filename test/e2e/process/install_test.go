//go:build e2e

package process_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hansehart/o3s/test/stacktest"
)

// pinned reads a value install.sh pins, by its variable's name.
func pinned(t *testing.T, script []byte, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^` + name + `="([^"]+)"$`).FindSubmatch(script)
	// Stop when the script pins no such value
	if m == nil {
		t.Fatalf("install.sh pins no %s", name)
	}
	return string(m[1])
}

// The devcontainer CLI's installer at the pinned tag still has the checksum install.sh pins, so a bump carries both.
func TestTheDevcontainersInstallerMatchesItsPin(t *testing.T) {
	script, err := os.ReadFile(filepath.Join(stacktest.Repo(t), "install.sh"))
	// Stop when the script fails to read
	if err != nil {
		t.Fatalf("ReadFile(install.sh) = %v", err)
	}
	version := pinned(t, script, "devcontainers_version")
	want := pinned(t, script, "devcontainers_installer_sha256")
	url := "https://raw.githubusercontent.com/devcontainers/cli/v" + version + "/scripts/install.sh"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	// Stop when the request fails to build
	if err != nil {
		t.Fatalf("NewRequest = %v", err)
	}

	resp, err := http.DefaultClient.Do(req)

	// Stop when the installer fails to download
	if err != nil {
		t.Fatalf("GET %s = %v", url, err)
	}
	defer resp.Body.Close()
	// Stop when GitHub serves no installer at the tag
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %s", url, resp.Status)
	}
	h := sha256.New()
	// Stop when the download breaks off
	if _, err := io.Copy(h, resp.Body); err != nil {
		t.Fatalf("read %s = %v", url, err)
	}
	// Report a checksum that differs from the pin
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		t.Errorf("installer at v%s has sha256 %s, install.sh pins %s", version, got, want)
	}
}

// Lima's checksums at the pinned release list every archive's checksum install.sh pins, so a bump carries all of them.
func TestTheLimaArchivesMatchTheirPins(t *testing.T) {
	script, err := os.ReadFile(filepath.Join(stacktest.Repo(t), "install.sh"))
	// Stop when the script fails to read
	if err != nil {
		t.Fatalf("ReadFile(install.sh) = %v", err)
	}
	version := pinned(t, script, "lima_version")
	url := "https://github.com/lima-vm/lima/releases/download/v" + version + "/SHA256SUMS"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	// Stop when the request fails to build
	if err != nil {
		t.Fatalf("NewRequest = %v", err)
	}

	resp, err := http.DefaultClient.Do(req)

	// Stop when the checksums fail to download
	if err != nil {
		t.Fatalf("GET %s = %v", url, err)
	}
	defer resp.Body.Close()
	// Stop when GitHub serves no checksums at the release
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %s", url, resp.Status)
	}
	sums, err := io.ReadAll(resp.Body)
	// Stop when the download breaks off
	if err != nil {
		t.Fatalf("read %s = %v", url, err)
	}
	archives := map[string]string{
		"lima_sha256_linux_amd64":  "Linux-x86_64",
		"lima_sha256_linux_arm64":  "Linux-aarch64",
		"lima_sha256_darwin_amd64": "Darwin-x86_64",
		"lima_sha256_darwin_arm64": "Darwin-arm64",
	}
	// Check every pinned archive against Lima's list
	for name, platform := range archives {
		line := pinned(t, script, name) + "  lima-" + version + "-" + platform + ".tar.gz"
		// Report a pin Lima's list lacks
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(line) + `$`).Match(sums) {
			t.Errorf("SHA256SUMS of Lima %s lacks %q", version, line)
		}
	}
}
