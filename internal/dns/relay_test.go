//go:build unix

package dns_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/hansehart/o3s/internal/dns"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/relay"
)

// The cage resolves a listed name over the relay's DNS socket.
func TestDNSServesTheCageOverTheRelay(t *testing.T) {
	ns := nameserver(t, addressAnswer())
	s := dns.New(dns.Options{Policy: listing(), Upstream: &servers{list: []string{ns.addr}}})
	dir, err := os.MkdirTemp("", "o3s") //nolint:usetesting // t.TempDir's long path outgrows a socket's path limit
	// Stop when the folder fails to appear
	if err != nil {
		t.Fatalf("Setup: MkdirTemp = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	u := relay.Unix{Dir: dir}
	r := relay.New(u)
	_ = r.Handle(relay.DNS, s.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		// Report a relay that fails to stop cleanly
		if err := <-done; err != nil {
			t.Errorf("Serve = %v, want a clean stop", err)
		}
	})
	c, err := dialRelay(t, u)
	// Stop when the relay keeps its socket shut
	if err != nil {
		t.Fatalf("Setup: Dial(%s) = %v", relay.DNS, err)
	}
	h := &home{cage: c}

	got := h.ask(t, query(t, fixture.Host, dnsmessage.TypeA))

	// Require the listed name answered
	if got.RCode != dnsmessage.RCodeSuccess || len(got.Answers) != 1 {
		t.Errorf("answer over the relay = %v with %d records, want success with one", got.RCode, len(got.Answers))
	}
}

// dialRelay reaches the relay's DNS socket once the relay listens on it.
func dialRelay(t *testing.T, u relay.Unix) (net.Conn, error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	// Try again until the relay listens or the time runs out
	for {
		c, err := u.Dial(t.Context(), relay.DNS)
		// Hand back the first connection, or the last failure once the time is up
		if err == nil || time.Now().After(deadline) {
			// Close the connection once the test ends
			if err == nil {
				t.Cleanup(func() { _ = c.Close() })
			}
			return c, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
