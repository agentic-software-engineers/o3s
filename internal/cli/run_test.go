package cli_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hansehart/o3s/internal/cli"
	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/home"
)

// aHome works inside a new home with an empty allowlist and an engine that runs no stack.
func aHome(t *testing.T) {
	t.Helper()
	engine := t.TempDir()
	// Stop when the engine stays unwritten
	if err := os.WriteFile(filepath.Join(engine, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil { //nolint:gosec // the test runs the fake engine
		t.Fatalf("Setup: %v", err)
	}
	t.Setenv("PATH", engine+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := t.TempDir()
	t.Setenv("HOME", home)
	c := config.Config{}
	// Stop when the home stays unwritten
	if err := c.Save(home); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Chdir(home)
}

// run hands a command line to the CLI and hands back its status and what it reported on stderr.
func run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut strings.Builder
	status := cli.Run(t.Context(), args, cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut})
	return status, errOut.String()
}

// The exit status tells a usage error apart from a failure and from success.
func TestRunExitsWithAStatusPerOutcome(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "success", args: []string{"firewall", "add", fixture.Host}, want: 0},
		{name: "usage error", args: []string{"firewall", "add"}, want: 2},
		{name: "rule broken", args: []string{"firewall", "add", "Example.com"}, want: 2},
		{name: "failure", args: []string{"firewall", "rm", fixture.AbsentName}, want: 1},
	}
	// Check every outcome in its own home
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aHome(t)

			got, errOut := run(t, tt.args...)

			// Require the status of the outcome
			if got != tt.want {
				t.Errorf("Run(%q) = %d, want %d, stderr:\n%s", tt.args, got, tt.want, errOut)
			}
		})
	}
}

// The file in the home is the one the command changed.
func TestRunWritesTheHomeItRunsIn(t *testing.T) {
	aHome(t)

	status, errOut := run(t, "firewall", "add", fixture.Host)

	// Stop when the command fails
	if status != 0 {
		t.Fatalf("Run(firewall add %s) = %d, stderr:\n%s", fixture.Host, status, errOut)
	}
	data, err := os.ReadFile(filepath.Join(".", config.File))
	// Require the host in the file
	if err != nil || !strings.Contains(string(data), fmt.Sprintf("%q", fixture.Host)) {
		t.Errorf("home file = %q, %v, want %s in it", data, err, fixture.Host)
	}
}

// Run hands the digests a release binary carries on to the stack it writes.
func TestRunPassesTheDigestsOn(t *testing.T) {
	digests := map[string]string{"ghcr.io/hansehart/o3s-cage": "sha256:" + strings.Repeat("a", 64)}
	cli.Digests = digests
	t.Cleanup(func() { cli.Digests, home.Digests = nil, nil })

	cli.Run(t.Context(), []string{"--version"}, cli.Streams{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard})

	// Require the digests in the home package
	if home.Digests["ghcr.io/hansehart/o3s-cage"] != digests["ghcr.io/hansehart/o3s-cage"] {
		t.Errorf("home.Digests = %v, want %v", home.Digests, digests)
	}
}

// A failure also lands in the file the menu names, so the menu still shows it once the terminal it printed on is gone.
func TestRunWritesItsErrorWhereTheMenuAsks(t *testing.T) {
	aHome(t)
	file := filepath.Join(t.TempDir(), "error")
	t.Setenv("O3S_ERROR_FILE", file)

	status, _ := run(t, "firewall", "rm", fixture.AbsentName)

	got, err := os.ReadFile(file)
	// Require the failure's own line in the file
	if status != 1 || err != nil || !strings.HasPrefix(string(got), "error: unknown host "+fixture.AbsentName) {
		t.Errorf("Run() = %d, file %q (%v), want 1 and the error line", status, got, err)
	}
}
