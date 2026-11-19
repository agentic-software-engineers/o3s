package fixture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
)

// The set's own import path, which production code leaves alone.
const self = "github.com/hansehart/o3s/internal/fixture"

// A test script reads every value of the set, each by a name of its own.
func TestEnvHandsOnEveryValue(t *testing.T) {
	values := constants(t)

	env := fixture.Env()

	// Report a count that differs, which names a value left out or handed twice
	if len(env) != len(values) {
		t.Errorf("Env() holds %d values, want %d, one per constant", len(env), len(values))
	}
	handed := map[string]bool{}
	// Note every value handed on
	for _, v := range env {
		handed[v] = true
	}
	// Require every constant among them
	for name, v := range values {
		// Report a constant a script never sees
		if !handed[v] {
			t.Errorf("Env() lacks %s (%q)", name, v)
		}
	}
}

// Every moment reads back as the text it comes from.
func TestTheMomentsMatchTheirText(t *testing.T) {
	moments := map[string]time.Time{fixture.NowText: fixture.Now, fixture.InAnHourText: fixture.InAnHour, fixture.InTwoHoursText: fixture.InTwoHours, fixture.LongAgoText: fixture.LongAgo, fixture.FarAheadText: fixture.FarAhead}

	// Check every moment against its text
	for text, at := range moments {
		// Report a moment that differs from its text
		if at.IsZero() || at.Format(time.RFC3339) != text {
			t.Errorf("moment %v, want %s", at, text)
		}
	}
}

// Only tests take values from the set, so nothing a user runs depends on it.
func TestOnlyTestsUseTheSet(t *testing.T) {
	root := moduleRoot(t)
	var users []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		// Hand back a walk that fails, and pass over folders that hold no code of the module
		if err != nil || d.IsDir() && (d.Name() == "testdata" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
			return err
		}
		// Pass over the helper packages only tests import, named for what they test as httptest is
		if d.IsDir() && strings.HasSuffix(d.Name(), "test") {
			return filepath.SkipDir
		}
		// Look at the module's Go code alone, its tests left out
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		// Hand back a file that fails to read
		if err != nil {
			return err
		}
		// Note a file that takes values from the set
		if slices.ContainsFunc(f.Imports, func(s *ast.ImportSpec) bool { return strings.Trim(s.Path.Value, `"`) == self }) {
			users = append(users, path)
		}
		return nil
	})

	// Stop when the module fails to read
	if err != nil {
		t.Fatalf("walk the module: %v", err)
	}
	// Report code a user runs that takes values from the set
	if len(users) > 0 {
		t.Errorf("code outside the tests imports %s: %v", self, users)
	}
}

// constants reads every constant of the set with its value as text, as a script sees it.
func constants(t *testing.T) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", nil, 0)
	// Stop when the set fails to read
	if err != nil {
		t.Fatalf("read fixture.go: %v", err)
	}
	out := map[string]string{}
	// Read every constant declaration
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		// Pass over anything but constants
		if !ok || gen.Tok != token.CONST {
			continue
		}
		// Take every constant with its literal value
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			lit := vs.Values[0].(*ast.BasicLit)
			value := lit.Value
			// Unquote a text value
			if lit.Kind == token.STRING {
				value, _ = strconv.Unquote(lit.Value)
			}
			out[vs.Names[0].Name] = value
		}
	}
	return out
}

// moduleRoot finds the folder that holds the module definition, above the test's own.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	// Stop when the working folder stays unknown
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Walk up until the module definition shows
	for {
		// Hand back the folder that holds it
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		// Stop at the top of the filesystem
		if parent == dir {
			t.Fatalf("Setup: find go.mod above %s", dir)
		}
		dir = parent
	}
}
