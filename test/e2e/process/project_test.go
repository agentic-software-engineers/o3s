//go:build e2e

package process_test

import (
	"os"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/test/stacktest"
)

// helloWorld is a small public repository, which the cage clones over HTTPS through the gateway and the proxy.
const (
	helloWorld = "https://github.com/octocat/Hello-World"
	helloPath  = "/home/ubuntu/projects/github.com/octocat/Hello-World"
)

// A repository cloned through the gateway lands in the cage by host, owner and name, joins the workspace as
// owner/name, and leaves the cage and the workspace with rm.
func TestAProjectIsClonedListedAndRemovedInTheCage(t *testing.T) {
	_, err := stacktest.O3s(t, "firewall", "add", "github.com", "--port", "443")
	// Stop when the host stays closed
	if err != nil {
		t.Fatalf("o3s firewall add = %v", err)
	}
	raised, err := stacktest.DevcontainerUp(t)
	// Stop when the stack fails to come up
	if err != nil {
		t.Fatalf("DevcontainerUp() = %v, want the stack raised", err)
	}
	// Stop when a clone an earlier run left in the projects volume stays
	if _, err := stacktest.ContainerExec(t, raised.ContainerID, "rm", "-rf", helloPath); err != nil {
		t.Fatalf("Setup: clear %s: %v", helloPath, err)
	}

	out, err := stacktest.O3s(t, "project", "add", helloWorld)

	// Stop when the clone fails
	if err != nil {
		t.Fatalf("o3s project add = %v", err)
	}
	// Report a clone that names another project
	if !strings.Contains(out, "Added octocat/Hello-World") {
		t.Errorf("o3s project add printed %q, want Added octocat/Hello-World", out)
	}
	// Report a clone whose files are missing in the cage
	if _, err := stacktest.ContainerExec(t, raised.ContainerID, "test", "-f", helloPath+"/README"); err != nil {
		t.Errorf("README in the cage's clone: %v", err)
	}
	listed, err := stacktest.O3s(t, "project")
	// Report a list that misses the project
	if err != nil || !strings.Contains(listed, "octocat/Hello-World  "+helloPath) {
		t.Errorf("o3s project = %q, %v, want octocat/Hello-World at %s", listed, err, helloPath)
	}
	// Stop when the removal fails
	if _, err := stacktest.O3s(t, "project", "rm", "octocat/Hello-World", "--yes"); err != nil {
		t.Fatalf("o3s project rm = %v", err)
	}
	// Report a folder the removal left in the cage
	if _, err := stacktest.ContainerExec(t, raised.ContainerID, "test", "-e", helloPath); err == nil {
		t.Errorf("%s in the cage after rm, want it gone", helloPath)
	}
	workspace, err := os.ReadFile(home.Workspace(stacktest.Home(t)))
	// Report a workspace that still lists the project
	if err != nil || strings.Contains(string(workspace), helloPath) {
		t.Errorf("workspace after rm = %q, %v, want it without %s", workspace, err, helloPath)
	}
}

// A clone over SSH in a cage that lacks an SSH agent adds the folder and names the clone to run.
func TestAProjectOverSSHWithoutAnAgentNamesTheClone(t *testing.T) {
	// Leave a host with an agent to the developer, whose devcontainer CLI may forward it into the cage
	if os.Getenv("SSH_AUTH_SOCK") != "" {
		t.Skip("SSH_AUTH_SOCK is set, so the cage may reach an agent")
	}
	_, err := stacktest.O3s(t, "firewall", "add", "github.com", "--port", "22")
	// Stop when the host stays closed
	if err != nil {
		t.Fatalf("o3s firewall add = %v", err)
	}
	// Stop when the stack fails to come up
	if _, err := stacktest.DevcontainerUp(t); err != nil {
		t.Fatalf("DevcontainerUp() = %v, want the stack raised", err)
	}

	out, err := stacktest.O3s(t, "project", "add", "octocat/Hello-World")

	// Stop when the add fails
	if err != nil {
		t.Fatalf("o3s project add = %v", err)
	}
	// Report a missing note on the clone to run
	if !strings.Contains(out, "git clone git@github.com:octocat/Hello-World.git "+helloPath) {
		t.Errorf("o3s project add printed %q, want the clone to run by hand", out)
	}
}
