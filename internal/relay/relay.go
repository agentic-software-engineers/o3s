// Package relay carries the cage's connections to the services o3s runs for it on the host, over the one transport
// the cage can reach: a Unix socket for a home outside a VM, vsock for a home inside one.
package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"slices"
	"sync"
	"time"
)

// Service names what a connection from the cage asks for on the host.
type Service string

// The services the host offers the cage.
const (
	// Proxy carries the cage's web traffic to the allow-list proxy.
	Proxy Service = "proxy"
	// DNS carries the cage's name lookups.
	DNS Service = "dns"
	// SSH carries the editor's shell to the cage.
	SSH Service = "ssh"
)

// services lists every service the relay knows.
var services = []Service{Proxy, DNS, SSH}

var (
	// ErrUnknownService reports a service outside the relay's set.
	ErrUnknownService = errors.New("unknown service")
	// ErrServiceTaken reports a service that already has a handler.
	ErrServiceTaken = errors.New("service already has a handler")
	// ErrClosed reports a relay that has already served and stopped.
	ErrClosed = errors.New("relay stopped")
)

// Waits between attempts to listen again, growing from the first to the last.
const (
	firstWait = 50 * time.Millisecond
	lastWait  = 2 * time.Second
)

// known checks that the relay offers the service.
func known(s Service) error {
	// Turn away a service outside the relay's set
	if !slices.Contains(services, s) {
		return fmt.Errorf("%w: %q", ErrUnknownService, s)
	}
	return nil
}

// Transport reaches the services from the cage's side and listens for them on the host's side.
type Transport interface {
	// Listen waits on the host's side for connections to one service.
	Listen(Service) (net.Listener, error)
	// Dial connects to one service, as the cage does.
	Dial(context.Context, Service) (net.Conn, error)
}

// Handler serves one connection from the cage, which the relay closes once the handler returns.
type Handler func(ctx context.Context, c net.Conn)

// Relay accepts the cage's connections for each service and hands each to the service's handler.
type Relay struct {
	transport Transport

	mu       sync.Mutex
	handlers map[Service]Handler
	started  bool
	stopping bool
	conns    map[net.Conn]struct{}
}

// New makes a relay over the transport, before it handles any service.
func New(t Transport) *Relay {
	return &Relay{transport: t, handlers: map[Service]Handler{}, conns: map[net.Conn]struct{}{}}
}

// Handle hands the service's connections to the handler.
func (r *Relay) Handle(s Service, h Handler) error {
	// Turn away a service outside the relay's set
	if err := known(s); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Turn away a second handler for the same service
	if _, ok := r.handlers[s]; ok {
		return fmt.Errorf("%w: %s", ErrServiceTaken, s)
	}
	r.handlers[s] = h
	return nil
}

// Forward carries the service's connections to the target the dial reaches, both ways, until both ends finish.
func (r *Relay) Forward(s Service, dial func(context.Context) (net.Conn, error)) error {
	return r.Handle(s, func(ctx context.Context, c net.Conn) {
		target, err := dial(ctx)
		// Drop the connection when its target is out of reach
		if err != nil {
			return
		}
		// Drop the target too when the relay is already stopping
		if !r.track(target) {
			_ = target.Close()
			return
		}
		defer r.untrack(target)
		defer target.Close()
		pipe(c, target)
	})
}

// Serve listens for every handled service until the context ends, then closes what it carries and returns.
func (r *Relay) Serve(ctx context.Context) error {
	r.mu.Lock()
	// Turn away a relay that has served already
	if r.started {
		r.mu.Unlock()
		return ErrClosed
	}
	r.started = true
	handlers := maps.Clone(r.handlers)
	r.mu.Unlock()

	var wg sync.WaitGroup
	// Listen for each service on its own
	for s, h := range handlers {
		wg.Go(func() { r.serve(ctx, &wg, s, h) })
	}
	<-ctx.Done()
	r.closeAll()
	wg.Wait()
	return nil
}

// serve listens for one service, and listens again whenever its listener ends, until the context ends.
func (r *Relay) serve(ctx context.Context, wg *sync.WaitGroup, s Service, h Handler) {
	var wait time.Duration
	// Keep the service listening until the relay stops
	for ctx.Err() == nil {
		l, err := r.transport.Listen(s)
		// Wait a growing while before trying again when the service fails to listen
		if err != nil {
			wait = grow(wait)
			// Stop when the relay stops while waiting
			if !pause(ctx, wait) {
				return
			}
			continue
		}
		// Start the waits afresh after a listener that carried a connection, or wait longer after one that stayed idle
		if r.accept(ctx, wg, l, h) {
			wait = 0
		} else {
			wait = grow(wait)
		}
		// Stop when the relay stops while waiting
		if !pause(ctx, wait) {
			return
		}
	}
}

// accept hands every connection the listener takes to the handler until the listener ends, and reports whether it
// took any.
func (r *Relay) accept(ctx context.Context, wg *sync.WaitGroup, l net.Listener, h Handler) bool {
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()
	defer l.Close()
	took := false
	// Take connections until the listener ends
	for {
		c, err := l.Accept()
		// Hand control back once the listener ends
		if err != nil {
			return took
		}
		took = true
		// Turn the connection away when the relay is already stopping
		if !r.track(c) {
			_ = c.Close()
			return took
		}
		wg.Go(func() {
			defer r.untrack(c)
			defer c.Close()
			h(ctx, c)
		})
	}
}

// track records an open connection so a stop can close it, and refuses it once the relay is stopping.
func (r *Relay) track(c net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Refuse new connections once the relay stops
	if r.stopping {
		return false
	}
	r.conns[c] = struct{}{}
	return true
}

// untrack forgets a connection that has closed.
func (r *Relay) untrack(c net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, c)
}

// closeAll closes every connection the relay carries, so their handlers return.
func (r *Relay) closeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopping = true
	// Close each open connection
	for c := range r.conns {
		_ = c.Close()
	}
}

// grow lengthens a wait, from the first wait up to the last.
func grow(wait time.Duration) time.Duration {
	return min(max(2*wait, firstWait), lastWait)
}

// pause waits for the given while and reports false when the context ends first.
func pause(ctx context.Context, wait time.Duration) bool {
	// Go on at once when the wait is zero
	if wait == 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	// Wait for the timer, or for the context to end first
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// closeWriter is a connection that can end its writing side alone.
type closeWriter interface {
	CloseWrite() error
}

// pipe copies both ways until both directions end.
func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Go(func() { copyHalf(b, a) })
	wg.Go(func() { copyHalf(a, b) })
	wg.Wait()
}

// copyHalf copies one direction to its end, then ends the writing side, or the whole connection where it fails to end
// one side alone.
func copyHalf(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	// Pass the end on as a half-close where the connection allows it
	if cw, ok := dst.(closeWriter); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = dst.Close()
}
