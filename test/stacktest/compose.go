package stacktest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Repo finds the root of this checkout, the directory that holds the module definition.
func Repo(tb testing.TB) string {
	tb.Helper()
	dir, err := repoRoot()
	// Stop when the checkout's root stays unfound
	if err != nil {
		tb.Fatalf("Setup: %v", err)
	}
	return dir
}

// repoRoot walks up from the working directory to the first directory that holds the module definition.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	// Hand back a working directory that is unreadable
	if err != nil {
		return "", fmt.Errorf("read the working directory: %w", err)
	}
	// Walk up from the working directory to the root of the checkout
	for {
		// Hand back the first directory that holds the module definition
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		// Hand back a walk that reaches the filesystem root
		if parent == dir {
			return "", errors.New("find go.mod above the working directory")
		}
		dir = parent
	}
}

// compose runs docker compose on the stack of the test's home with the dev overlay, and hands back what it printed.
func compose(ctx context.Context, tb testing.TB, args ...string) (string, error) {
	tb.Helper()
	files := []string{
		"compose",
		"-f", filepath.Join(aHome(tb), ".devcontainer", "compose.yaml"),
		"-f", filepath.Join(Repo(tb), "test", "stacktest", "compose.dev.yaml"),
	}
	return run(ctx, tb, "docker", append(files, args...)...)
}

// Up raises the services in the test's home on the cage image built from this commit and waits until they are healthy. It removes them once
// the test ends.
func Up(tb testing.TB, services ...string) error {
	tb.Helper()
	tb.Cleanup(func() {
		// Report a service that outlives its test
		if _, err := compose(context.WithoutCancel(tb.Context()), tb, append([]string{"rm", "--stop", "--force"}, services...)...); err != nil {
			tb.Errorf("Cleanup: %v", err)
		}
	})
	_, err := compose(tb.Context(), tb, append([]string{"up", "--wait", "--wait-timeout", "60"}, services...)...)
	return err
}

// Exec runs a command in a running service and hands back what it printed.
func Exec(tb testing.TB, service string, command ...string) (string, error) {
	tb.Helper()
	return compose(tb.Context(), tb, append([]string{"exec", "-T", service}, command...)...)
}

// Compose runs docker compose on the stack of the test's home with the dev overlay, such as stop or start on one of
// its services, and hands back what it printed.
func Compose(tb testing.TB, args ...string) (string, error) {
	tb.Helper()
	return compose(tb.Context(), tb, args...)
}
