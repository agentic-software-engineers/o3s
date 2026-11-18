// Package ide opens an o3s home's workspace in VS Code straight in its cage.
package ide

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/tailscale/hujson"
)

// workspaceFolder is where the cage mounts the home, as the stack's devcontainer.json names it.
const workspaceFolder = "/home/ubuntu/o3s"

// URI is the address VS Code opens a home's workspace at: the host's folder in hex, the SSH host VS Code reaches it
// through where there is one, and the workspace inside the cage.
func URI(hostPath, sshHost, workspace string) string {
	authority := "dev-container+" + hex.EncodeToString([]byte(hostPath))
	// Open the cage on the machine VS Code reaches over SSH
	if sshHost != "" {
		authority += "@ssh-remote+" + sshHost
	}
	return "vscode-remote://" + authority + path.Join(workspaceFolder, workspace)
}

// Env is the environment o3s runs in, as VS Code sees it.
type Env interface {
	// HostPath is the home's folder the way VS Code names it.
	HostPath(ctx context.Context, home string) (string, error)
	// Check names what VS Code needs before it opens the cage.
	Check(ctx context.Context) error
	// SSHHost is the name VS Code reaches the machine by over SSH, and empty where VS Code runs on it.
	SSHHost() string
}

// Detect finds the environment o3s runs in: a session over SSH, WSL where it names its distribution, and Linux
// otherwise.
func Detect() Env {
	// Take a session over SSH, which VS Code reaches through the host it connected to
	if os.Getenv("SSH_CONNECTION") != "" {
		return remote{host: os.Getenv("O3S_SSH_HOST"), window: os.Getenv("VSCODE_IPC_HOOK_CLI") != ""}
	}
	// Take WSL where it names its distribution
	if distro := os.Getenv("WSL_DISTRO_NAME"); distro != "" {
		return wsl{distro: distro}
	}
	return linux{}
}

// linux is a Linux host, where VS Code sees the folder as it is.
type linux struct{}

// HostPath hands back the folder as it is.
func (linux) HostPath(_ context.Context, home string) (string, error) {
	return home, nil
}

// Check passes, since VS Code runs Docker right on the host.
func (linux) Check(context.Context) error {
	return nil
}

// SSHHost is empty, since VS Code runs on the machine itself.
func (linux) SSHHost() string {
	return ""
}

// remote is a machine reached over SSH, where a VS Code Remote-SSH window opens the cage through the host it
// connected to.
type remote struct {
	// host is the name the window connected with.
	host string
	// window reports whether the terminal belongs to a VS Code window, which its code command reaches.
	window bool
}

// HostPath hands back the folder as it is, since the cage runs on the same machine.
func (remote) HostPath(_ context.Context, home string) (string, error) {
	return home, nil
}

// Check passes in a VS Code Remote-SSH terminal that names the host the window connected with, and names what to
// set otherwise.
func (r remote) Check(context.Context) error {
	// Point at the terminal whose code command reaches a VS Code window
	if !r.window {
		return errors.New("o3s cage ide over SSH runs in the terminal of a VS Code Remote-SSH window")
	}
	// Point at the variable that names the host the window connected with
	if r.host == "" {
		return errors.New("o3s cage ide over SSH reads the host VS Code connected with from O3S_SSH_HOST, " +
			"the name after SSH: in its status bar, e.g. export O3S_SSH_HOST=myvm")
	}
	return nil
}

// SSHHost is the name the window connected with.
func (r remote) SSHHost() string {
	return r.host
}

// wsl is a WSL distribution, where VS Code runs on the Windows side.
type wsl struct {
	distro string
}

// HostPath hands back the folder's Windows form.
func (wsl) HostPath(ctx context.Context, home string) (string, error) {
	return output(ctx, "wslpath", "-w", home)
}

// Check passes where VS Code's settings let the Dev Containers extension run Docker in WSL, and names the lines to add otherwise.
func (w wsl) Check(ctx context.Context) error {
	path, err := settingsPath(ctx)
	// Pass where the settings hold the line
	if err == nil && executesInWSL(path) {
		return nil
	}
	return fmt.Errorf("VS Code runs Docker inside WSL with these lines in its settings.json:\n"+
		"  \"dev.containers.executeInWSL\": true,\n"+
		"  \"dev.containers.executeInWSLDistro\": %q", w.distro)
}

// SSHHost is empty, since VS Code reaches WSL on the same machine.
func (wsl) SSHHost() string {
	return ""
}

// settingsPath finds VS Code's user settings on the Windows side of WSL.
func settingsPath(ctx context.Context) (string, error) {
	appData, err := output(ctx, "cmd.exe", "/c", "echo %APPDATA%")
	// Hand back a Windows folder that stays unknown
	if err != nil {
		return "", err
	}
	dir, err := output(ctx, "wslpath", "-u", appData)
	// Hand back a Windows folder WSL fails to name
	if err != nil {
		return "", err
	}
	return path.Join(dir, "Code", "User", "settings.json"), nil
}

// executesInWSL reports whether VS Code's settings let the Dev Containers extension run Docker in WSL.
func executesInWSL(settingsPath string) bool {
	data, err := os.ReadFile(settingsPath)
	// Read settings that fail to read as unset
	if err != nil {
		return false
	}
	plain, err := hujson.Standardize(data)
	// Read settings that fail to parse as unset
	if err != nil {
		return false
	}
	var settings map[string]any
	// Read settings in another shape as unset
	if json.Unmarshal(plain, &settings) != nil {
		return false
	}
	on, _ := settings["dev.containers.executeInWSL"].(bool)
	return on
}

// output runs a command and hands back what it printed, trimmed of line ends.
func output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out)), err
}
