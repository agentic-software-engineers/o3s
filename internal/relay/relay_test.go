package relay_test

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/relay"
)

// serving runs the relay for the rest of the test and stops it once the test ends.
func serving(t *testing.T, r *relay.Relay) {
	t.Helper()
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
}

// dialing reaches the service once the relay listens for it, failing the test once the time runs out.
func dialing(t *testing.T, tr relay.Transport, s relay.Service) net.Conn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	// Try again until the relay listens or the time runs out
	for {
		c, err := tr.Dial(t.Context(), s)
		// Hand back the first connection that gets through
		if err == nil {
			t.Cleanup(func() { _ = c.Close() })
			return c
		}
		// Stop once the relay has had time enough to listen
		if time.Now().After(deadline) {
			t.Fatalf("Setup: Dial(%s) = %v, want the relay listening", s, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// replying is a handler that reads the cage's message, hands it to the test, and answers with the reply.
func replying(got chan<- string) relay.Handler {
	return func(_ context.Context, c net.Conn) {
		b := make([]byte, len(fixture.Message))
		_, _ = io.ReadFull(c, b)
		got <- string(b)
		_, _ = c.Write([]byte(fixture.Reply))
	}
}

// answering is a handler that answers every connection with the given words.
func answering(words string) relay.Handler {
	return func(_ context.Context, c net.Conn) {
		_, _ = c.Write([]byte(words))
	}
}

// readAll reads a connection until the other side closes it.
func readAll(t *testing.T, c net.Conn) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	b, err := io.ReadAll(c)
	// Report a connection that fails before it ends
	if err != nil {
		t.Errorf("read = %v, want the stream until its end", err)
	}
	return string(b)
}

// The cage's bytes reach the service's handler, and the handler's answer comes back.
func TestRelayCarriesBytesBothWays(t *testing.T) {
	tr := newPipeTransport(t)
	r := relay.New(tr)
	got := make(chan string, 1)
	_ = r.Handle(relay.Proxy, replying(got))
	serving(t, r)

	c := dialing(t, tr, relay.Proxy)
	_, _ = c.Write([]byte(fixture.Message))
	reply := readAll(t, c)

	// Require both directions to carry their bytes
	if diff := cmp.Diff([]string{fixture.Message, fixture.Reply}, []string{<-got, reply}); diff != "" {
		t.Errorf("relay carried (-want +got):\n%s", diff)
	}
}

// Each service's connections reach that service's own handler.
func TestRelayRoutesEachServiceToItsHandler(t *testing.T) {
	tr := newPipeTransport(t)
	r := relay.New(tr)
	_ = r.Handle(relay.Proxy, answering(string(relay.Proxy)))
	_ = r.Handle(relay.DNS, answering(string(relay.DNS)))
	serving(t, r)

	got := map[relay.Service]string{
		relay.Proxy: readAll(t, dialing(t, tr, relay.Proxy)),
		relay.DNS:   readAll(t, dialing(t, tr, relay.DNS)),
	}

	want := map[relay.Service]string{relay.Proxy: string(relay.Proxy), relay.DNS: string(relay.DNS)}
	// Require every service to answer from its own handler
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("answers by service (-want +got):\n%s", diff)
	}
}

// A forwarded service carries the cage's bytes to the target its dial reaches.
func TestRelayForwardsToTheDialTarget(t *testing.T) {
	tr := newPipeTransport(t)
	r := relay.New(tr)
	near, far := net.Pipe()
	t.Cleanup(func() { _ = far.Close() })
	_ = r.Forward(relay.SSH, func(context.Context) (net.Conn, error) { return near, nil })
	serving(t, r)

	c := dialing(t, tr, relay.SSH)
	go func() { _, _ = c.Write([]byte(fixture.Message)) }()
	b := make([]byte, len(fixture.Message))
	_ = far.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err := io.ReadFull(far, b)

	// Require the target to read what the cage wrote
	if err != nil || string(b) != fixture.Message {
		t.Errorf("target read %q, %v, want %q", b, err, fixture.Message)
	}
}

// A service outside the relay's set is turned away.
func TestHandleRefusesAnUnknownService(t *testing.T) {
	r := relay.New(newPipeTransport(t))

	err := r.Handle(relay.Service(fixture.AbsentName), answering(fixture.Reply))

	// Require the refusal to name the unknown service
	if !errors.Is(err, relay.ErrUnknownService) {
		t.Errorf("Handle(%s) = %v, want ErrUnknownService", fixture.AbsentName, err)
	}
}

// A service that already has a handler is turned away the second time.
func TestHandleRefusesAServiceTwice(t *testing.T) {
	r := relay.New(newPipeTransport(t))
	_ = r.Handle(relay.DNS, answering(fixture.Reply))

	err := r.Handle(relay.DNS, answering(fixture.Reply))

	// Require the second handler to be refused
	if !errors.Is(err, relay.ErrServiceTaken) {
		t.Errorf("Handle(%s) twice = %v, want ErrServiceTaken", relay.DNS, err)
	}
}

// Ending the context stops the relay and ends the connections it carries.
func TestServeStopsWhenTheContextEnds(t *testing.T) {
	tr := newPipeTransport(t)
	r := relay.New(tr)
	_ = r.Handle(relay.Proxy, func(ctx context.Context, _ net.Conn) { <-ctx.Done() })
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx) }()
	c := dialing(t, tr, relay.Proxy)

	cancel()

	// Require Serve to return cleanly, and the open connection to end
	select {
	case err := <-done:
		// Report a stop that claims a failure
		if err != nil {
			t.Errorf("Serve after cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve still runs after its context ended")
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	// Report a connection the stop left open
	if _, err := c.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read after stop = %v, want EOF", err)
	}
}

// A relay that has stopped refuses to serve again.
func TestServeRefusesAfterItStopped(t *testing.T) {
	r := relay.New(newPipeTransport(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_ = r.Serve(ctx)

	err := r.Serve(t.Context())

	// Require the second Serve to be refused
	if !errors.Is(err, relay.ErrClosed) {
		t.Errorf("Serve after stop = %v, want ErrClosed", err)
	}
}

// A connection that ends early leaves the next one served.
func TestServeKeepsServingAfterAConnectionEnds(t *testing.T) {
	tr := newPipeTransport(t)
	r := relay.New(tr)
	_ = r.Handle(relay.Proxy, answering(fixture.Reply))
	serving(t, r)
	_ = dialing(t, tr, relay.Proxy).Close()

	got := readAll(t, dialing(t, tr, relay.Proxy))

	// Require the second connection to get its answer
	if got != fixture.Reply {
		t.Errorf("second connection read %q, want %q", got, fixture.Reply)
	}
}
