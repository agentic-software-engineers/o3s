package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/devcontainer"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/home"
	"github.com/hansehart/o3s/internal/menu"
	"github.com/hansehart/o3s/internal/registry"
	"github.com/hansehart/o3s/internal/registry/registrytest"
)

// The menu knows every secret the home names or stores, whether it holds a value, and the home's stores.
func TestMenuHomeReadsTheHome(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	secretsDir := t.TempDir()
	t.Setenv("O3S_SECRETS_DIR", secretsDir)
	settings := fmt.Sprintf(`{"$schema": "./.devcontainer/o3s.schema.json", "stores": [{"id": %q}], "hosts": [
  {"name": %q, "ports": [%d], "secret": %q, "inject": ["header"], "by": ["user"]}
]}`, fixture.Store, fixture.Host, fixture.Port, fixture.Secret)
	// Stop when the settings stay unwritten
	if err := os.WriteFile(filepath.Join(dir, "o3s.json"), []byte(settings), 0o600); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the secrets stay unwritten
	if err := os.WriteFile(filepath.Join(secretsDir, "secrets.env"), []byte(fixture.OtherSecret+"="+fixture.SecretValue+"\n"), 0o600); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	got := menuHome(t.Context())

	got.Lookup = nil
	want := menu.Home{
		Dir:     dir,
		Exists:  true,
		Hosts:   got.Hosts,
		Secrets: []menu.Secret{{Name: fixture.Secret}, {Name: fixture.OtherSecret, Set: true}},
		Stores:  []string{fixture.Store},
	}
	// Report a home read otherwise
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("menuHome() (-want +got):\n%s", diff)
	}
	// Require the host the settings name
	if len(got.Hosts) != 1 || got.Hosts[0].Name != fixture.Host {
		t.Errorf("menuHome() hosts = %v, want %s", got.Hosts, fixture.Host)
	}
}

// Outside a home the menu still starts, knowing only where it runs.
func TestMenuHomeStartsOutsideAHome(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("O3S_SECRETS_DIR", t.TempDir())

	got := menuHome(t.Context())

	got.Lookup = nil
	// Require a home that names the folder alone
	if diff := cmp.Diff(menu.Home{Dir: dir}, got); diff != "" {
		t.Errorf("menuHome() (-want +got):\n%s", diff)
	}
}

// aStoreWithTheTool raises a store that offers the tool and an other feature, and hands back the store.
func aStoreWithTheTool(t *testing.T) string {
	t.Helper()
	r, err := registrytest.New()
	// Stop when the registry fails to come up
	if err != nil {
		t.Fatalf("Setup: registrytest.New() = %v", err)
	}
	t.Cleanup(r.Close)
	tool := fmt.Sprintf(`{"id": %q, "version": %q, "customizations": {"o3s": {
  "hosts": [{"name": %q, "ports": [%d], "secret": %q}],
  "env": [{"name": "EXAMPLE_MODE", "value": "strict"}]
}}}`, fixture.Feature, fixture.Version, fixture.Host, fixture.Port, fixture.Secret)
	// Stop when the store fails to publish
	if err := r.PushFeatures("features", tool, fmt.Sprintf(`{"id": %q}`, fixture.OtherFeature)); err != nil {
		t.Fatalf("Setup: PushFeatures() = %v", err)
	}
	return r.Host + "/features"
}

// aHomeIn makes a new home the test works in, with secrets of its own.
func aHomeIn(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("O3S_SECRETS_DIR", t.TempDir())
	// Stop when the home stays unwritten
	if _, err := home.Init(dir); err != nil {
		t.Fatalf("Setup: Init() = %v", err)
	}
	return dir
}

// The menu reads a store's features and shows what adding one brings, as feature add prints it.
func TestMenuLookupShowsWhatAFeatureBrings(t *testing.T) {
	store := aStoreWithTheTool(t)
	aHomeIn(t)
	l := menuLookup{ctx: t.Context(), r: &registry.Resolver{}}

	ids, err := l.Features(store)
	// Stop when the store fails to read
	if err != nil {
		t.Fatalf("Features() = %v", err)
	}
	pv, err := l.Add(store + "/" + fixture.Feature)
	// Stop when the preview fails
	if err != nil {
		t.Fatalf("Add() = %v", err)
	}

	// Require the store's ids in order
	if diff := cmp.Diff([]string{fixture.OtherFeature, fixture.Feature}, ids); diff != "" {
		t.Errorf("Features() (-want +got):\n%s", diff)
	}
	want := menu.Preview{
		Lines:   []string{store + "/" + fixture.Feature + ":" + fixture.Major, fmt.Sprintf("  + %s:%d  %s !", fixture.Host, fixture.Port, fixture.Secret), "  + EXAMPLE_MODE=strict"},
		Waiting: []string{fixture.Secret},
	}
	// Require the lines feature add prints
	if diff := cmp.Diff(want, pv); diff != "" {
		t.Errorf("Add() (-want +got):\n%s", diff)
	}
}

// The menu shows what leaves with an installed feature, and lists the features the home holds.
func TestMenuLookupShowsWhatLeavesWithAFeature(t *testing.T) {
	store := aStoreWithTheTool(t)
	dir := aHomeIn(t)
	pkg := store + "/" + fixture.Feature
	c, err := config.Load(dir)
	// Stop when the settings fail to load
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the tool's host stays out
	if err := c.Need(pkg, config.Change{Name: fixture.Host, Secret: fixture.Secret}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	dc, err := devcontainer.Load(dir)
	// Stop when the config fails to load
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	dc.SetFeature(pkg, pkg+":"+fixture.Major, nil)
	dc.SetEnv("EXAMPLE_MODE", "strict")
	// Stop when the home fails to save
	if err := saveHome(dir, c, dc); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	l := menuLookup{ctx: t.Context(), r: &registry.Resolver{}}

	pv, err := l.Remove(pkg)

	// Stop when the preview fails
	if err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	// Require the lines feature rm prints
	if diff := cmp.Diff(menu.Preview{Lines: []string{pkg, fmt.Sprintf("  - %s:%d", fixture.Host, fixture.Port), "  - EXAMPLE_MODE=strict"}}, pv); diff != "" {
		t.Errorf("Remove() (-want +got):\n%s", diff)
	}
	// Require the installed feature on offer
	if diff := cmp.Diff([]string{pkg}, menuHome(t.Context()).Features); diff != "" {
		t.Errorf("menuHome().Features (-want +got):\n%s", diff)
	}
}
