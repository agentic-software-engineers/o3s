//go:build unix

package proxy_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/relay"
)

// relaying serves the proxy over a relay on Unix sockets for the rest of the test, and hands back the transport the
// cage dials.
func relaying(t *testing.T, h relay.Handler) relay.Unix {
	t.Helper()
	dir, err := os.MkdirTemp("", "o3s") //nolint:usetesting // t.TempDir's long path outgrows a socket's path limit
	// Stop when no folder can be made
	if err != nil {
		t.Fatalf("Setup: MkdirTemp = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	u := relay.Unix{Dir: dir}
	r := relay.New(u)
	_ = r.Handle(relay.Proxy, h)
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
	return u
}

// The cage reaches a listed host through the proxy the relay serves on its socket.
func TestProxyServesTheCageOverTheRelay(t *testing.T) {
	p, up, _ := proxying(t, listed(fixture.Port))
	s := replying(t, true)
	up.route(fixture.Port, s)
	u := relaying(t, p.Handle)
	c := client(t, p, s)
	c.Transport.(*http.Transport).DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return u.Dial(ctx, relay.Proxy)
	}

	status, body := eventually(t, func() (int, string) { return get(t, c, "https://"+fixture.Host+"/") })

	// Require the host's own answer
	if status != http.StatusOK || body != fixture.Reply {
		t.Errorf("GET https://%s over the relay = %d %q, want %d %q", fixture.Host, status, body, http.StatusOK, fixture.Reply)
	}
}

// eventually fetches until an answer comes, since the relay starts listening a moment after it starts serving.
func eventually(t *testing.T, fetch func() (int, string)) (int, string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	// Try again until an answer comes or the time runs out
	for {
		status, body := fetch()
		// Hand back the first answer, or the last failure once the time is up
		if status != 0 || time.Now().After(deadline) {
			return status, body
		}
		time.Sleep(10 * time.Millisecond)
	}
}
