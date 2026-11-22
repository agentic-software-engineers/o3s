package relay_test

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/relay"
)

// errNoListener is the fake transport's answer to a dial that lacks a listener.
var errNoListener = errors.New("no listener for the service")

// pipeTransport is an in-memory transport for the relay's tests, held to the real one by the transport contract.
type pipeTransport struct {
	mu        sync.Mutex
	listeners map[relay.Service]*pipeListener
}

// newPipeTransport hands a test an in-memory transport before any listener joins.
func newPipeTransport(*testing.T) relay.Transport {
	return &pipeTransport{listeners: map[relay.Service]*pipeListener{}}
}

func (p *pipeTransport) Listen(s relay.Service) (net.Listener, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	l := &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
	l.close = func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		// Forget the listener alone, leaving a later one for the same service in place
		if p.listeners[s] == l {
			delete(p.listeners, s)
		}
	}
	p.listeners[s] = l
	return l, nil
}

func (p *pipeTransport) Dial(ctx context.Context, s relay.Service) (net.Conn, error) {
	p.mu.Lock()
	l, ok := p.listeners[s]
	p.mu.Unlock()
	// Refuse a service that lacks a listener
	if !ok {
		return nil, errNoListener
	}
	near, far := net.Pipe()
	// Hand the far end to the listener while it stays open and the dial keeps waiting
	select {
	case l.conns <- far:
		return near, nil
	case <-l.done:
		return nil, errNoListener
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// pipeListener hands out the far ends of the pipes its transport dials.
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
	close func()
}

func (l *pipeListener) Accept() (net.Conn, error) {
	// Hand out the next pipe, or end once the listener closes
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() {
		close(l.done)
		l.close()
	})
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

// pipeAddr names the in-memory transport's listeners.
type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

// holdsTransportContract checks what every transport promises the relay.
func holdsTransportContract(t *testing.T, newTransport func(*testing.T) relay.Transport) {
	t.Helper()

	// A dial reaches the listener of its service, carrying bytes to it.
	t.Run("a dial reaches the listener of its service", func(t *testing.T) {
		tr := newTransport(t)
		l, err := tr.Listen(relay.Proxy)
		// Stop when the service fails to listen
		if err != nil {
			t.Fatalf("Setup: Listen(%s) = %v", relay.Proxy, err)
		}
		t.Cleanup(func() { _ = l.Close() })
		got := make(chan string, 1)
		go func() {
			c, err := l.Accept()
			// Report an empty string when the connection fails to arrive
			if err != nil {
				got <- ""
				return
			}
			defer c.Close()
			b := make([]byte, len(fixture.Message))
			_, _ = io.ReadFull(c, b)
			got <- string(b)
		}()

		c, err := tr.Dial(t.Context(), relay.Proxy)
		// Stop when the dial fails
		if err != nil {
			t.Fatalf("Dial(%s) = %v, want a connection", relay.Proxy, err)
		}
		defer c.Close()
		_, _ = c.Write([]byte(fixture.Message))

		// Require the listener to read what the dialer wrote
		if g := <-got; g != fixture.Message {
			t.Errorf("listener read %q, want %q", g, fixture.Message)
		}
	})

	// A dial to a service that lacks a listener fails.
	t.Run("a dial to a service nobody listens on fails", func(t *testing.T) {
		tr := newTransport(t)

		_, err := tr.Dial(t.Context(), relay.DNS)

		// Require the dial to fail
		if err == nil {
			t.Errorf("Dial(%s) with no listener = nil, want an error", relay.DNS)
		}
	})

	// Closing a listener ends a pending Accept.
	t.Run("closing a listener ends its Accept", func(t *testing.T) {
		tr := newTransport(t)
		l, err := tr.Listen(relay.SSH)
		// Stop when the service fails to listen
		if err != nil {
			t.Fatalf("Setup: Listen(%s) = %v", relay.SSH, err)
		}
		ended := make(chan error, 1)
		go func() {
			_, err := l.Accept()
			ended <- err
		}()

		_ = l.Close()

		// Require Accept to return an error soon after the close
		select {
		case err := <-ended:
			// Report an Accept that claims a connection
			if err == nil {
				t.Error("Accept after Close = nil, want an error")
			}
		case <-time.After(5 * time.Second):
			t.Error("Accept still waits after Close")
		}
	})
}

// The in-memory transport the relay's tests use keeps the transport contract.
func TestPipeTransportHoldsTheContract(t *testing.T) {
	holdsTransportContract(t, newPipeTransport)
}
