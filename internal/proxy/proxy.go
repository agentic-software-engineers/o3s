// Package proxy is the allow-list proxy the cage reaches the internet through: it opens a tunnel or forwards plain web
// traffic to a host the allowlist holds on that port, and refuses every other before it reaches out.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"
)

// plainPort is the port plain web traffic takes when its address names none.
const plainPort = 80

// Timeouts that keep a slow or silent peer from holding the proxy.
const (
	dialTimeout   = 30 * time.Second
	headerTimeout = 30 * time.Second
	continueWait  = time.Second
	idleTimeout   = 90 * time.Second
)

// errBadTarget reports an address the proxy cannot read a host and port from.
var errBadTarget = errors.New("no host and port")

// Policy decides which hosts the cage may reach, and on which ports.
type Policy interface {
	// Allows reports whether the host may be reached on the port.
	Allows(host string, port int) bool
}

// Options are what the proxy works with.
type Options struct {
	// Policy decides which hosts the cage may reach.
	Policy Policy
	// Dial reaches a host the policy lets through, and a plain dialer does when it is left empty.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// Log takes a line for every refusal, and nothing is logged when it is left empty.
	Log *slog.Logger
}

// Proxy serves the cage's connections one by one, as the relay hands them over.
type Proxy struct {
	policy Policy
	dial   func(ctx context.Context, network, addr string) (net.Conn, error)
	log    *slog.Logger
	ctx    context.Context
	stop   context.CancelFunc
	conns  *handed
	srv    *http.Server
	fwd    *httputil.ReverseProxy
}

// New starts a proxy with the options.
func New(o Options) *Proxy {
	ctx, stop := context.WithCancel(context.Background())
	p := &Proxy{policy: o.Policy, dial: o.Dial, log: o.Log, ctx: ctx, stop: stop, conns: newHanded()}
	// Reach hosts directly when no dial is given
	if p.dial == nil {
		d := &net.Dialer{Timeout: dialTimeout}
		p.dial = d.DialContext
	}
	// Drop the log when none is given
	if p.log == nil {
		p.log = slog.New(slog.DiscardHandler)
	}
	p.fwd = &httputil.ReverseProxy{
		Rewrite: rewrite,
		Transport: &http.Transport{
			DialContext:           p.dial,
			ExpectContinueTimeout: continueWait,
			IdleConnTimeout:       idleTimeout,
		},
		ErrorHandler: p.unreachable,
	}
	p.srv = &http.Server{
		Handler:           p,
		ReadHeaderTimeout: headerTimeout,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ErrorLog:          slog.NewLogLogger(p.log.Handler(), slog.LevelDebug),
	}
	go func() { _ = p.srv.Serve(p.conns) }()
	return p
}

// Handle serves one connection from the cage until it ends, the context ends or the proxy closes.
func (p *Proxy) Handle(ctx context.Context, c net.Conn) {
	h := &held{Conn: c, closed: make(chan struct{})}
	// Hand the connection to the server, unless the proxy or the context ends first
	select {
	case p.conns.ch <- h:
	case <-p.ctx.Done():
		return
	case <-ctx.Done():
		return
	}
	// Wait for the server to finish with the connection
	select {
	case <-h.closed:
	case <-p.ctx.Done():
	case <-ctx.Done():
	}
}

// Close stops the proxy and ends every connection it serves.
func (p *Proxy) Close() error {
	p.stop()
	return p.srv.Close()
}

// ServeHTTP opens a tunnel for a CONNECT and forwards every other request.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Open a tunnel where the cage asks for one
	if r.Method == http.MethodConnect {
		p.tunnel(w, r)
		return
	}
	p.forward(w, r)
}

// tunnel connects the cage to a listed host and carries bytes both ways until both sides finish.
func (p *Proxy) tunnel(w http.ResponseWriter, r *http.Request) {
	host, port, err := target(r.Host, 0)
	// Turn away a tunnel that names no host and port
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Turn away a host the policy does not let through
	if !p.allows(w, host, port) {
		return
	}
	up, err := p.dial(r.Context(), "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	// Report a listed host that cannot be reached
	if err != nil {
		p.unreachable(w, r, err)
		return
	}
	defer up.Close()
	c, rw, err := http.NewResponseController(w).Hijack()
	// Give up on a connection the server will not hand over
	if err != nil {
		return
	}
	defer c.Close()
	stop := context.AfterFunc(p.ctx, func() { _ = c.Close(); _ = up.Close() })
	defer stop()
	_, _ = rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
	// Stop when the cage is gone before the tunnel opens
	if err := rw.Flush(); err != nil {
		return
	}
	var wg sync.WaitGroup
	wg.Go(func() { carry(up, rw.Reader) })
	carry(c, up)
	wg.Wait()
}

// forward sends a plain request to a listed host and its answer back.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request) {
	host, port, err := target(r.URL.Host, plainPort)
	// Turn away a request meant for a server rather than a proxy, or for a scheme other than plain web traffic
	if r.URL.Scheme != "http" || err != nil {
		http.Error(w, "a proxy request names its http:// host", http.StatusBadRequest)
		return
	}
	// Turn away a host the policy does not let through
	if !p.allows(w, host, port) {
		return
	}
	p.fwd.ServeHTTP(w, r)
}

// allows checks the host and port against the policy, and refuses the request where the policy does not let it through.
func (p *Proxy) allows(w http.ResponseWriter, host string, port int) bool {
	// Let through what the policy allows
	if p.policy != nil && p.policy.Allows(host, port) {
		return true
	}
	p.log.Info("refused", "host", host, "port", port)
	http.Error(w, fmt.Sprintf("o3s refused %s, which the allowlist does not hold", net.JoinHostPort(host, strconv.Itoa(port))), http.StatusForbidden)
	return false
}

// unreachable reports a listed host the proxy cannot reach.
func (p *Proxy) unreachable(w http.ResponseWriter, r *http.Request, err error) {
	p.log.Warn("unreachable", "host", r.Host, "err", err)
	w.WriteHeader(http.StatusBadGateway)
}

// rewrite sends a forwarded request to the host and port its address names, spelled as the policy checked them.
func rewrite(pr *httputil.ProxyRequest) {
	host, port, _ := target(pr.In.URL.Host, plainPort)
	pr.Out.URL.Host = net.JoinHostPort(host, strconv.Itoa(port))
}

// target reads the host and port from an address, with the port it falls back to where the address names none, and
// spells the host as the allowlist does.
func target(addr string, fallback int) (string, int, error) {
	host, portText, err := net.SplitHostPort(addr)
	// Take the whole address as the host where it names no port and a port to fall back to is known
	if err != nil && fallback != 0 {
		host, portText, err = addr, strconv.Itoa(fallback), nil
	}
	// Turn away an address with no port to fall back to
	if err != nil {
		return "", 0, fmt.Errorf("%w in %q", errBadTarget, addr)
	}
	port, err := strconv.Atoi(portText)
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	// Turn away a port that does not exist, or an empty host
	if err != nil || port < 1 || port > 65535 || host == "" {
		return "", 0, fmt.Errorf("%w in %q", errBadTarget, addr)
	}
	return host, port, nil
}

// closeWriter is a connection that can end its writing side alone.
type closeWriter interface {
	CloseWrite() error
}

// carry copies one direction to its end, then ends the writing side, or the whole connection where it cannot end one
// side alone.
func carry(dst net.Conn, src io.Reader) {
	_, _ = io.Copy(dst, src)
	// Pass the end on as a half-close where the connection allows it
	if cw, ok := dst.(closeWriter); ok && cw.CloseWrite() == nil {
		return
	}
	_ = dst.Close()
}

// held is a connection the server serves, which tells Handle once the server closes it.
type held struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

// Close closes the connection and tells Handle it is done.
func (h *held) Close() error {
	h.once.Do(func() { close(h.closed) })
	return h.Conn.Close()
}

// CloseWrite ends the writing side alone where the connection beneath allows it.
func (h *held) CloseWrite() error {
	// Pass the half-close on where the connection beneath has one
	if cw, ok := h.Conn.(closeWriter); ok {
		return cw.CloseWrite()
	}
	return errors.ErrUnsupported
}

// handed is the listener the server takes the relay's connections from.
type handed struct {
	ch   chan net.Conn
	done chan struct{}
	once sync.Once
}

// newHanded makes a listener with no connection handed over yet.
func newHanded() *handed {
	return &handed{ch: make(chan net.Conn), done: make(chan struct{})}
}

// Accept waits for the next connection Handle hands over.
func (l *handed) Accept() (net.Conn, error) {
	// Wait for a connection, unless the listener closes first
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

// Close stops the listener taking connections.
func (l *handed) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

// Addr names the listener, which has no address of its own.
func (l *handed) Addr() net.Addr {
	return handedAddr{}
}

// handedAddr is the address of a listener that takes connections in memory.
type handedAddr struct{}

// Network names the kind of address.
func (handedAddr) Network() string { return "relay" }

// String names the address.
func (handedAddr) String() string { return "relay" }
