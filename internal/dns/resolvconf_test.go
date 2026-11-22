package dns_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/dns"
	"github.com/hansehart/o3s/internal/fixture"
)

// writeResolvConf puts the text where the reader looks for it.
func writeResolvConf(t *testing.T, path, text string) {
	t.Helper()
	// Stop when the file stays unwritten
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("Setup: WriteFile = %v", err)
	}
}

// The reader takes every nameserver line in order, IPv6 included, and skips comments and the other options.
func TestResolvConfReadsTheNameservers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	writeResolvConf(t, path, "# generated\nsearch "+fixture.Host+"\nnameserver "+fixture.Address+"\n; old\nnameserver "+fixture.AddressV6+"\noptions ndots:1\n")
	r := &dns.ResolvConf{Path: path}

	got, err := r.Nameservers()

	want := []string{net.JoinHostPort(fixture.Address, "53"), net.JoinHostPort(fixture.AddressV6, "53")}
	// Require both nameservers in their order
	if diff := cmp.Diff(want, got); diff != "" || err != nil {
		t.Errorf("Nameservers() = %v (-want +got):\n%s", err, diff)
	}
}

// The reader reads the file afresh each time, so a changed file applies at once.
func TestResolvConfFollowsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	writeResolvConf(t, path, "nameserver "+fixture.Address+"\n")
	r := &dns.ResolvConf{Path: path}
	_, _ = r.Nameservers()

	writeResolvConf(t, path, "nameserver "+fixture.AddressV6+"\n")
	got, _ := r.Nameservers()

	// Require the new nameserver
	if diff := cmp.Diff([]string{net.JoinHostPort(fixture.AddressV6, "53")}, got); diff != "" {
		t.Errorf("Nameservers() after the change (-want +got):\n%s", diff)
	}
}

// A file that goes missing or empty for a moment leaves the last good list in use.
func TestResolvConfKeepsTheLastGoodList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	writeResolvConf(t, path, "nameserver "+fixture.Address+"\n")
	r := &dns.ResolvConf{Path: path}
	_, _ = r.Nameservers()
	want := []string{net.JoinHostPort(fixture.Address, "53")}

	_ = os.Remove(path)
	gone, _ := r.Nameservers()
	writeResolvConf(t, path, "# rewriting\n")
	empty, _ := r.Nameservers()

	// Require the last good list both times
	if diff := cmp.Diff([][]string{want, want}, [][]string{gone, empty}); diff != "" {
		t.Errorf("Nameservers() while the file is gone, then empty (-want +got):\n%s", diff)
	}
}

// A reader still waiting for its first good file reports the failure.
func TestResolvConfReportsAMissingFile(t *testing.T) {
	r := &dns.ResolvConf{Path: filepath.Join(t.TempDir(), "resolv.conf")}

	_, err := r.Nameservers()

	// Require the failure
	if err == nil {
		t.Error("Nameservers() of a missing file = nil, want an error")
	}
}
