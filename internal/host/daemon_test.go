package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aDaemon puts a fake docker on PATH that answers info with the security options given, or fails.
func aDaemon(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	// Stop when the fake stays unwritten
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil { //nolint:gosec // the test runs its fake
		t.Fatalf("Setup: %v", err)
	}
	t.Setenv("PATH", bin)
}

// The daemon's own security options say whether it confines containers with AppArmor.
func TestDaemonAppArmorAsksDocker(t *testing.T) {
	tests := []struct {
		name, options string
		want          bool
	}{
		{"a daemon with AppArmor", `["name=apparmor","name=seccomp,profile=builtin","name=cgroupns"]`, true},
		{"a daemon without it", `["name=seccomp,profile=builtin","name=cgroupns"]`, false},
	}
	// Ask every daemon
	for _, tt := range tests {
		aDaemon(t, "echo '"+tt.options+"'")

		got, err := DaemonAppArmor(t.Context())

		// Report an answer that differs
		if err != nil || got != tt.want {
			t.Errorf("%s: DaemonAppArmor() = %v, %v, want %v", tt.name, got, err, tt.want)
		}
	}
}

// A daemon out of reach is an error that names Docker.
func TestDaemonAppArmorNamesADaemonOutOfReach(t *testing.T) {
	aDaemon(t, "echo 'Cannot connect to the Docker daemon' >&2; exit 1")

	_, err := DaemonAppArmor(t.Context())

	// Require an error that names Docker
	if err == nil || !strings.Contains(err.Error(), "Docker") {
		t.Errorf("DaemonAppArmor() = %v, want an error naming Docker", err)
	}
}

// The values compose reads name the cage's AppArmor profile, or unconfined where the home opts out.
func TestEnvNamesTheProfile(t *testing.T) {
	// Write the values for each profile
	for _, profile := range []string{"o3s-cage", "unconfined"} {
		home := t.TempDir()
		// Stop when the stack folder fails to appear
		if err := os.MkdirAll(filepath.Join(home, ".devcontainer"), 0o750); err != nil {
			t.Fatalf("Setup: %v", err)
		}

		err := Env(home, profile)

		data, _ := os.ReadFile(filepath.Join(home, ".devcontainer", ".env"))
		// Require the profile among the values
		if err != nil || !strings.Contains(string(data), "O3S_APPARMOR_PROFILE="+profile+"\n") {
			t.Errorf("Env(%s) = %v:\n%s", profile, err, data)
		}
	}
}
