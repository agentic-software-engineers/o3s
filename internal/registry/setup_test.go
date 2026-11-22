package registry_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/registry"
	"github.com/hansehart/o3s/internal/registry/registrytest"
)

// aStoreWithTheTool hands a test a registry whose store offers the tool, and the tool's id there.
func aStoreWithTheTool(t *testing.T) (*registrytest.Registry, registry.Ref) {
	t.Helper()
	r := aRegistry(t)
	// Stop when the store fails to publish
	if err := r.PushFeatures("features", aTool); err != nil {
		t.Fatalf("Setup: PushFeatures(features) = %v", err)
	}
	return r, registry.Ref{Store: r.Host + "/features", ID: fixture.Feature}
}

// A feature resolves to the store's major version, its state dir, and its env with the state dir filled in.
func TestResolveWorksOutWhatAFeatureSetsUp(t *testing.T) {
	_, ref := aStoreWithTheTool(t)
	var r registry.Resolver

	got, err := r.Resolve(t.Context(), ref)

	// Stop when the feature fails to resolve
	if err != nil {
		t.Fatalf("Resolve(tool) = %v", err)
	}
	dir := "/home/ubuntu/features/" + ref.Package()
	want := registry.Setup{
		Ref:     registry.Ref{Store: ref.Store, ID: fixture.Feature, Tag: fixture.Major},
		Version: fixture.Version,
		Options: map[string]any{"stateDir": dir},
		Offered: []registry.Option{
			{Name: "channel", Type: "string", Default: "stable", Enum: []string{"stable", "beta"}},
			{Name: "debug", Type: "boolean", Default: "false", Description: "Log every step"},
			{Name: "stateDir", Type: "string"},
			{Name: "version", Type: "string", Default: fixture.FloatingTag, Proposals: []string{fixture.FloatingTag, "lts"}, Description: "Which release to install"},
		},
		Hosts: []registry.Host{{Name: fixture.Host, Ports: []int{fixture.Port}, Secret: fixture.Secret, Inject: []string{"header"}}},
		Env:   []registry.Env{{Name: "EXAMPLE_HOME", Value: dir}},
	}
	// Report a setup that differs
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Resolve(tool) (-want +got):\n%s", diff)
	}
}

// A tag the id names stays as it is.
func TestResolveKeepsTheTagTheIDNames(t *testing.T) {
	_, ref := aStoreWithTheTool(t)
	ref.Tag = fixture.MinorTag
	var r registry.Resolver

	got, err := r.Resolve(t.Context(), ref)

	// Report a tag that moved
	if err != nil || got.Ref.Tag != fixture.MinorTag {
		t.Errorf("Resolve(%s) = %v, %v, want the tag %s", ref, got.Ref, err, fixture.MinorTag)
	}
}

// A feature the store lacks is named as unknown.
func TestResolveTurnsAwayAnUnknownFeature(t *testing.T) {
	_, ref := aStoreWithTheTool(t)
	ref.ID = "absent"
	var r registry.Resolver

	_, err := r.Resolve(t.Context(), ref)

	// Require the error that names an unknown feature
	if !errors.Is(err, registry.ErrUnknownFeature) {
		t.Errorf("Resolve(absent) = %v, want ErrUnknownFeature", err)
	}
}

// A resolver reads each store once, so its features stay at hand once the registry is gone.
func TestResolverReadsEachStoreOnce(t *testing.T) {
	reg, ref := aStoreWithTheTool(t)
	var r registry.Resolver
	// Stop when the first read fails
	if _, err := r.Resolve(t.Context(), ref); err != nil {
		t.Fatalf("Setup: Resolve(tool) = %v", err)
	}
	reg.Close()

	_, err := r.Resolve(t.Context(), ref)

	// Require the feature from what the resolver read before
	if err != nil {
		t.Errorf("Resolve(tool) after the registry closed = %v, want it from the first read", err)
	}
}

// A feature that lacks a version follows the newest release.
func TestMajorReadsTheMajorPart(t *testing.T) {
	// Check every version
	for in, want := range map[string]string{fixture.NewerVersion: fixture.NewerMajor, "3": "3", "": fixture.FloatingTag} {
		// Report a major part that differs
		if got := registry.Major(in); got != want {
			t.Errorf("Major(%q) = %q, want %q", in, got, want)
		}
	}
}
