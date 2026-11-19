package secrets_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/secrets"
)

// header is the comment block a secrets file opens with.
const header = "# NAME\n#        secrets.env — real API tokens for the egress proxy\n"

// aSecretsFile hands a test a secrets file holding the given content, and its path.
func aSecretsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "o3s", secrets.File)
	// Stop when the folder stays uncreated
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the file stays unwritten
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return path
}

// load reads a secrets file, stopping the test when it fails.
func load(t *testing.T, path string) secrets.Store {
	t.Helper()
	s, err := secrets.Load(path)
	// Stop when the file stays unread
	if err != nil {
		t.Fatalf("Load(%q) = %v", path, err)
	}
	return s
}

// Reading follows the proxy, the last set value winning. A value with a line break counts as none.
func TestValueReadsLikeTheProxy(t *testing.T) {
	path := aSecretsFile(t, header+fmt.Sprintf("\n%[1]s = \"first\"\n%[1]s='second'\n%[1]s=\nBROKEN=a\rb\n", fixture.Secret))
	s := load(t, path)

	got, ok := s.Value(fixture.Secret)
	broken, brokenOK := s.Value("BROKEN")

	// Require the last value that holds anything
	if !ok || got != "second" {
		t.Errorf("Value(%s) = %q, %v, want %q, true", fixture.Secret, got, ok, "second")
	}
	// Require a value with a line break to count as none
	if brokenOK || broken != "" {
		t.Errorf("Value(BROKEN) = %q, %v, want none", broken, brokenOK)
	}
}

// Setting a name fills a line that holds it without a value, and keeps every other line as it was.
func TestSetFillsALineWithoutAValue(t *testing.T) {
	path := aSecretsFile(t, header+fixture.Secret+"=\n")
	s := load(t, path)

	replaced, err := s.Set(fixture.Secret, fixture.SecretValue)

	// Stop when the value is turned away
	if err != nil {
		t.Fatalf("Set(%s) = %v", fixture.Secret, err)
	}
	// Require a line without a value to count as no earlier value
	if replaced {
		t.Errorf("Set(%s) replaced = true, want false for a line without a value", fixture.Secret)
	}
	// Require the line filled and the header kept
	if diff := cmp.Diff(header+fixture.Secret+"="+fixture.SecretValue+"\n", s.String()); diff != "" {
		t.Errorf("Set(%s) file (-want +got):\n%s", fixture.Secret, diff)
	}
}

// Setting a name that holds a value replaces it, drops later lines that could shadow it, and says so.
func TestSetReplacesAValueAndItsShadows(t *testing.T) {
	path := aSecretsFile(t, header+fmt.Sprintf("%[1]s=old\n%[1]s=older\n", fixture.Secret))
	s := load(t, path)

	replaced, err := s.Set(fixture.Secret, "new")

	// Stop when the value is turned away
	if err != nil {
		t.Fatalf("Set(%s) = %v", fixture.Secret, err)
	}
	// Require the rotation reported
	if !replaced {
		t.Errorf("Set(%s) replaced = false, want true", fixture.Secret)
	}
	// Require one line holding the new value
	if diff := cmp.Diff(header+fixture.Secret+"=new\n", s.String()); diff != "" {
		t.Errorf("Set(%s) file (-want +got):\n%s", fixture.Secret, diff)
	}
}

// Setting a name the file lacks appends it.
func TestSetAppendsANewName(t *testing.T) {
	path := aSecretsFile(t, header)
	s := load(t, path)

	_, err := s.Set(fixture.Secret, fixture.SecretValue)

	// Stop when the value is turned away
	if err != nil {
		t.Fatalf("Set(%s) = %v", fixture.Secret, err)
	}
	// Require the name after the header
	if diff := cmp.Diff(header+fixture.Secret+"="+fixture.SecretValue+"\n", s.String()); diff != "" {
		t.Errorf("Set(%s) file (-want +got):\n%s", fixture.Secret, diff)
	}
}

// Setting a value the proxy would refuse, or a name outside the rules, is turned away.
func TestSetTurnsAwayWhatTheProxyRefuses(t *testing.T) {
	tests := []struct {
		name, secret, value string
	}{
		{name: "empty value", secret: fixture.Secret, value: "  "},
		{name: "line break", secret: fixture.Secret, value: "a\nb"},
		{name: "carriage return", secret: fixture.Secret, value: "a\rb"},
		{name: "lowercase name", secret: strings.ToLower(fixture.Secret), value: fixture.SecretValue},
	}
	// Check every case against its own empty file
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := load(t, aSecretsFile(t, header))

			_, err := s.Set(tt.secret, tt.value)

			// Require the rule named
			if !errors.Is(err, secrets.ErrInvalid) {
				t.Errorf("Set(%q, %q) = %v, want ErrInvalid", tt.secret, tt.value, err)
			}
		})
	}
}

// Removing a name drops every line of it and keeps the rest.
func TestRemoveDropsEveryLineOfAName(t *testing.T) {
	path := aSecretsFile(t, header+fmt.Sprintf("%[1]s=a\n%[2]s=b\n%[1]s=\n", fixture.Secret, fixture.OtherSecret))
	s := load(t, path)

	ok := s.Remove(fixture.Secret)

	// Require the name gone and the rest kept
	if diff := cmp.Diff(header+fixture.OtherSecret+"=b\n", s.String()); !ok || diff != "" {
		t.Errorf("Remove(%s) = %v, file (-want +got):\n%s", fixture.Secret, ok, diff)
	}
}

// Names lists every name the file holds, with or without a value, in order.
func TestNamesListsEveryName(t *testing.T) {
	s := load(t, aSecretsFile(t, header+fmt.Sprintf("%s=b\n%s=\n", fixture.OtherSecret, fixture.Secret)))

	got := s.Names()

	// Require both names, sorted
	if diff := cmp.Diff([]string{fixture.Secret, fixture.OtherSecret}, got); diff != "" {
		t.Errorf("Names() (-want +got):\n%s", diff)
	}
}

// Saving writes the file for its owner alone, in a folder for its owner alone, and reads back the same.
func TestSaveKeepsTheFileToItsOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "o3s", secrets.File)
	s := load(t, path)
	// Stop when the value is turned away
	if _, err := s.Set(fixture.Secret, fixture.SecretValue); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err := s.Save(path)

	// Stop when the file stays unwritten
	if err != nil {
		t.Fatalf("Save(%q) = %v", path, err)
	}
	file, _ := os.Stat(path)
	dir, _ := os.Stat(filepath.Dir(path))
	// Require the owner alone to read the file and its folder
	if file.Mode().Perm() != 0o600 || dir.Mode().Perm() != 0o700 {
		t.Errorf("Save(%q) modes = file %v, folder %v, want 0600 and 0700", path, file.Mode().Perm(), dir.Mode().Perm())
	}
	got, _ := load(t, path).Value(fixture.Secret)
	// Require the value to read back
	if got != fixture.SecretValue {
		t.Errorf("Value after Save = %q, want %q", got, fixture.SecretValue)
	}
}
