package ide_test

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/ide"
)

// The URI names the host's folder in hex, the SSH host VS Code reaches it through, and the workspace inside the cage,
// as VS Code opens it.
func TestURIOpensTheWorkspaceInTheCage(t *testing.T) {
	tests := []struct{ host, sshHost, want string }{
		{"/home/ubuntu/o3s", "", "vscode-remote://dev-container+" + hex.EncodeToString([]byte("/home/ubuntu/o3s")) + "/home/ubuntu/o3s/o3s.code-workspace"},
		{`\\wsl.localhost\Ubuntu\home\ubuntu\o3s`, "", "vscode-remote://dev-container+" + hex.EncodeToString([]byte(`\\wsl.localhost\Ubuntu\home\ubuntu\o3s`)) + "/home/ubuntu/o3s/o3s.code-workspace"},
		{"/home/ubuntu/o3s", fixture.SSHHost, "vscode-remote://dev-container+" + hex.EncodeToString([]byte("/home/ubuntu/o3s")) + "@ssh-remote+" + fixture.SSHHost + "/home/ubuntu/o3s/o3s.code-workspace"},
	}
	// Check every host path
	for _, tt := range tests {
		// Report a URI that differs
		if got := ide.URI(tt.host, tt.sshHost, "o3s.code-workspace"); got != tt.want {
			t.Errorf("URI(%q, %q) = %q, want %q", tt.host, tt.sshHost, got, tt.want)
		}
	}
}

// fakes puts commands on PATH, each a shell script the test gives.
func fakes(t *testing.T, scripts map[string]string) {
	t.Helper()
	bin := t.TempDir()
	// Write every fake
	for name, script := range scripts {
		// Stop when the fake stays unwritten
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil { //nolint:gosec // the test runs its fakes
			t.Fatalf("Setup: %v", err)
		}
	}
	t.Setenv("PATH", bin)
}

// aWindows fakes WSL's Windows side, whose VS Code keeps the given settings, or none where they are empty.
func aWindows(t *testing.T, settings string) {
	t.Helper()
	appData := t.TempDir()
	fakes(t, map[string]string{
		"cmd.exe": `echo 'C:\Users\ubuntu\AppData\Roaming'`,
		"wslpath": `[ "$1" = -u ] && echo '` + appData + `'`,
	})
	// Leave VS Code without settings where the test gives none
	if settings == "" {
		return
	}
	dir := filepath.Join(appData, "Code", "User")
	// Stop when the settings stay unwritten
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the settings stay unwritten
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatalf("Setup: %v", err)
	}
}

// Under WSL the host path is the folder's Windows form, which the Dev Containers extension reads.
func TestHostPathTakesTheWindowsFormUnderWSL(t *testing.T) {
	fakes(t, map[string]string{"wslpath": `[ "$1" = -w ] && printf '%s\n' '\\wsl.localhost\Ubuntu'"$2"`})
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")

	got, err := ide.Detect().HostPath(t.Context(), "/home/ubuntu/o3s")

	// Report a path that differs
	if want := `\\wsl.localhost\Ubuntu/home/ubuntu/o3s`; err != nil || got != want {
		t.Errorf("HostPath() = %q, %v, want %q", got, err, want)
	}
}

// Outside WSL the host path is the folder as it is, and VS Code needs nothing first.
func TestLinuxKeepsTheFolderAndNeedsNothing(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("WSL_DISTRO_NAME", "")
	env := ide.Detect()

	got, err := env.HostPath(t.Context(), "/home/ubuntu/o3s")

	// Report a path that differs
	if err != nil || got != "/home/ubuntu/o3s" {
		t.Errorf("HostPath() = %q, %v, want the folder itself", got, err)
	}
	// Report a check that asks for something
	if err := env.Check(t.Context()); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
}

// Under WSL the check passes where VS Code's settings set executeInWSL true, and names both lines otherwise.
func TestWSLCheckReadsTheSetting(t *testing.T) {
	tests := []struct {
		name, settings string
		ok             bool
	}{
		{"set true, with comments", "{\n  // run the extension's docker in WSL\n  \"dev.containers.executeInWSL\": true,\n}\n", true},
		{"set false", `{"dev.containers.executeInWSL": false}`, false},
		{"missing", `{"editor.fontSize": 14}`, false},
		{"no settings", "", false},
	}
	// Check every settings file
	for _, tt := range tests {
		aWindows(t, tt.settings)
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("WSL_DISTRO_NAME", "Ubuntu")

		err := ide.Detect().Check(t.Context())

		// Report a setting VS Code holds that the check turns away
		if tt.ok && err != nil {
			t.Errorf("%s: Check() = %v, want nil", tt.name, err)
		}
		// Report a missing setting the check lets through, or names without both lines
		if !tt.ok && (err == nil || !strings.Contains(err.Error(), `"dev.containers.executeInWSL": true`) || !strings.Contains(err.Error(), `"dev.containers.executeInWSLDistro": "Ubuntu"`)) {
			t.Errorf("%s: Check() = %v, want both lines named", tt.name, err)
		}
	}
}

// aSession puts the test in a session over SSH, in a VS Code window's terminal or not, naming the given host.
func aSession(t *testing.T, window bool, host string) {
	t.Helper()
	t.Setenv("SSH_CONNECTION", fmt.Sprintf("%s %d %s %d", fixture.Address, fixture.Port, fixture.Address, fixture.OtherPort))
	t.Setenv("O3S_SSH_HOST", host)
	t.Setenv("VSCODE_IPC_HOOK_CLI", "")
	// Hand the terminal a VS Code window where the test asks for one
	if window {
		t.Setenv("VSCODE_IPC_HOOK_CLI", filepath.Join(t.TempDir(), "vscode-ipc.sock"))
	}
}

// Over SSH a Remote-SSH terminal that names its host opens the folder as it is, through that host.
func TestSSHOpensThroughTheHostTheWindowConnectedWith(t *testing.T) {
	aSession(t, true, fixture.SSHHost)
	env := ide.Detect()

	got, err := env.HostPath(t.Context(), "/home/ubuntu/o3s")

	// Report a path that differs
	if err != nil || got != "/home/ubuntu/o3s" {
		t.Errorf("HostPath() = %q, %v, want the folder itself", got, err)
	}
	// Report a check that asks for something
	if err := env.Check(t.Context()); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
	// Report a host that differs
	if got := env.SSHHost(); got != fixture.SSHHost {
		t.Errorf("SSHHost() = %q, want %q", got, fixture.SSHHost)
	}
}

// Over SSH the check names the Remote-SSH terminal outside one, and the variable for the host where it stays unset.
func TestSSHCheckNamesWhatIsMissing(t *testing.T) {
	tests := []struct {
		name   string
		window bool
		host   string
		want   string
	}{
		{"plain terminal", false, fixture.SSHHost, "Remote-SSH"},
		{"no host", true, "", "O3S_SSH_HOST"},
	}
	// Check every session
	for _, tt := range tests {
		aSession(t, tt.window, tt.host)

		err := ide.Detect().Check(t.Context())

		// Report a check that lets the session through or names something else
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: Check() = %v, want it to name %s", tt.name, err, tt.want)
		}
	}
}
