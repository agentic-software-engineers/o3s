//go:build integration

package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/secrets"
	"github.com/hansehart/o3s/test/stacktest"
)

// The proxy reads every host of the home's policy and the secret it injects for each.
func TestTheProxyReadsTheHomesPolicy(t *testing.T) {
	policy := fmt.Sprintf(`{
  "$schema": "./.devcontainer/o3s.schema.json",
  "stores": [],
  "hosts": [
    {"name": %q, "ports": [%d, %d], "by": ["user"]},
    {"name": %q, "ports": [%d], "secret": %q, "inject": ["header", "body"], "by": ["user"]},
    {"name": %q, "ports": [%d], "by": ["user"]}
  ]
}`, fixture.Host, fixture.OtherPort, fixture.Port, fixture.Subdomain, fixture.Port, fixture.Secret, fixture.OtherWildcard, fixture.Port)

	got, err := stacktest.ProxyReads(t, policy, fixture.Secret+"="+fixture.SecretValue+"\n")

	// Stop when the proxy fails on the policy
	if err != nil {
		t.Fatalf("ProxyReads(policy) = %v", err)
	}
	token := fixture.SecretValue
	wildcard := strings.TrimPrefix(fixture.OtherWildcard, "*")
	want := stacktest.Parsed{
		Keys:     []string{wildcard, fixture.Subdomain, fixture.Host},
		Allowed:  []string{wildcard, fixture.Subdomain, fixture.Host},
		Injected: map[string]*string{fixture.Subdomain: &token},
		Sites:    map[string][]string{fixture.Subdomain: {"body", "header"}},
	}
	// Require the hosts, the secret and its sites as the policy names them
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ProxyReads(policy) (-want +got):\n%s", diff)
	}
}

// The proxy injects the value o3s stores, in a file that held the name without a value.
func TestTheProxyReadsTheSecretsO3sStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), secrets.File)
	// Stop when the secrets file stays unwritten
	if err := os.WriteFile(path, []byte("# real API tokens for the egress proxy\n"+fixture.Secret+"=\n"), 0o600); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	store, err := secrets.Load(path)
	// Stop when the secrets file stays unread
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Stop when the value is turned away
	if _, err := store.Set(fixture.Secret, fixture.SecretValue); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	policy := fmt.Sprintf(`{"hosts": [{"name": %q, "ports": [%d], "secret": %q, "by": ["user"]}]}`, fixture.Subdomain, fixture.Port, fixture.Secret)

	got, err := stacktest.ProxyReads(t, policy, store.String())

	// Stop when the proxy fails on the stored secrets
	if err != nil {
		t.Fatalf("ProxyReads(policy, stored secrets) = %v", err)
	}
	token := fixture.SecretValue
	// Require the stored value injected for the host that carries it
	if diff := cmp.Diff(map[string]*string{fixture.Subdomain: &token}, got.Injected); diff != "" {
		t.Errorf("ProxyReads injected (-want +got):\n%s", diff)
	}
}
