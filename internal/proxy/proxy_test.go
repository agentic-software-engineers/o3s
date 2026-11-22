package proxy_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/proxy"
)

// plainPort is the port plain web traffic takes.
const plainPort = 80

// upstreams stands in for the internet: each name and port it knows leads to a test server, and every dial is noted.
type upstreams struct {
	mu     sync.Mutex
	routes map[string]string
	dialed []string
}

// route sends the listed host on the port to the test server.
func (u *upstreams) route(port int, srv *httptest.Server) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.routes[net.JoinHostPort(fixture.Host, strconv.Itoa(port))] = srv.Listener.Addr().String()
}

// dial reaches the test server the address leads to, and fails for an address it does not know.
func (u *upstreams) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	u.mu.Lock()
	u.dialed = append(u.dialed, addr)
	to, ok := u.routes[addr]
	u.mu.Unlock()
	// Fail as an unreachable host would
	if !ok {
		return nil, fmt.Errorf("dial %s: no route to host", addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, network, to)
}

// dials hands back every address the proxy dialed.
func (u *upstreams) dials() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.dialed...)
}

// proxying starts a proxy that lets the hosts through, over upstreams no test server answers yet, and logs to the
// buffer.
func proxying(t *testing.T, hosts ...config.Host) (*proxy.Proxy, *upstreams, *bytes.Buffer) {
	t.Helper()
	u := &upstreams{routes: map[string]string{}}
	var log bytes.Buffer
	p := proxy.New(proxy.Options{
		Policy: &config.Config{Hosts: hosts},
		Dial:   u.dial,
		Log:    slog.New(slog.NewJSONHandler(&syncWriter{w: &log}, nil)),
	})
	t.Cleanup(func() { _ = p.Close() })
	return p, u, &log
}

// syncWriter lets the proxy's goroutines write the log one at a time.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// Write writes the bytes while no other write runs.
func (s *syncWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(b)
}

// connect hands back a connection to the proxy, as the relay would hand the proxy the cage's connection.
func connect(t *testing.T, p *proxy.Proxy) net.Conn {
	t.Helper()
	cage, host := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer host.Close()
		p.Handle(ctx, host)
	}()
	t.Cleanup(func() {
		cancel()
		_ = cage.Close()
		<-done
	})
	return cage
}

// client is a web client in the cage that goes through the proxy and trusts the test servers.
func client(t *testing.T, p *proxy.Proxy, servers ...*httptest.Server) *http.Client {
	t.Helper()
	roots := x509.NewCertPool()
	// Trust each test server's certificate
	for _, s := range servers {
		roots.AddCert(s.Certificate())
	}
	tr := &http.Transport{
		Proxy:             http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.invalid"}),
		DialContext:       func(context.Context, string, string) (net.Conn, error) { return connect(t, p), nil },
		TLSClientConfig:   &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
	}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 5 * time.Second}
}

// replying is a test server that answers every request with the reply.
func replying(t *testing.T, tlsOn bool) *httptest.Server {
	t.Helper()
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture.Reply)
	}))
	// Serve over TLS with HTTP/2 where the test asks
	if tlsOn {
		s.EnableHTTP2 = true
		s.StartTLS()
	} else {
		s.Start()
	}
	t.Cleanup(s.Close)
	return s
}

// listed is the host the allowlist lets through on the ports.
func listed(ports ...int) config.Host {
	return config.Host{Name: fixture.Host, Ports: ports}
}

// get fetches the address and hands back its status and body.
func get(t *testing.T, c *http.Client, addr string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, addr, nil)
	// Stop when the request cannot be built
	if err != nil {
		t.Fatalf("Setup: NewRequest(%s) = %v", addr, err)
	}
	resp, err := c.Do(req)
	// Report a fetch that fails before any answer
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// rawRequest writes a request to the proxy as the cage would, and hands back the answer's status with the connection
// and its reader for what follows.
func rawRequest(t *testing.T, p *proxy.Proxy, request string) (int, *bufio.Reader, net.Conn) {
	t.Helper()
	c := connect(t, p)
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	go func() { _, _ = io.WriteString(c, request) }()
	r := bufio.NewReader(c)
	resp, err := http.ReadResponse(r, nil)
	// Stop when the proxy gives no answer
	if err != nil {
		t.Fatalf("read the proxy's answer = %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp.StatusCode, r, c
}

// A listed host answers through the tunnel the cage opens to it.
func TestProxyTunnelsToAListedHost(t *testing.T) {
	p, u, _ := proxying(t, listed(fixture.Port))
	s := replying(t, true)
	u.route(fixture.Port, s)

	status, body := get(t, client(t, p, s), "https://"+fixture.Host+"/")

	// Require the host's own answer
	if status != http.StatusOK || body != fixture.Reply {
		t.Errorf("GET https://%s = %d %q, want %d %q", fixture.Host, status, body, http.StatusOK, fixture.Reply)
	}
}

// A host the allowlist lacks is refused before the proxy reaches out to it.
func TestProxyRefusesATunnelToAnUnlistedHost(t *testing.T) {
	p, u, _ := proxying(t, listed(fixture.Port))
	addr := net.JoinHostPort(fixture.RefusedName, strconv.Itoa(fixture.Port))

	status, _, _ := rawRequest(t, p, "CONNECT "+addr+" HTTP/1.1\r\nHost: "+addr+"\r\n\r\n")

	// Require a refusal and no dial at all
	if status != http.StatusForbidden || len(u.dials()) != 0 {
		t.Errorf("CONNECT %s = %d with dials %v, want %d and none", addr, status, u.dials(), http.StatusForbidden)
	}
}

// A listed host is refused on a port the allowlist does not open for it.
func TestProxyRefusesAListedHostOnAnotherPort(t *testing.T) {
	p, u, _ := proxying(t, listed(fixture.Port))
	addr := net.JoinHostPort(fixture.Host, strconv.Itoa(fixture.OtherPort))

	status, _, _ := rawRequest(t, p, "CONNECT "+addr+" HTTP/1.1\r\nHost: "+addr+"\r\n\r\n")

	// Require a refusal and no dial at all
	if status != http.StatusForbidden || len(u.dials()) != 0 {
		t.Errorf("CONNECT %s = %d with dials %v, want %d and none", addr, status, u.dials(), http.StatusForbidden)
	}
}

// A name in capitals or with its closing dot matches the host the allowlist names.
func TestProxyMatchesANameInAnySpelling(t *testing.T) {
	p, u, _ := proxying(t, listed(fixture.Port))
	s := replying(t, true)
	u.route(fixture.Port, s)
	addr := net.JoinHostPort(strings.ToUpper(fixture.Host)+".", strconv.Itoa(fixture.Port))

	status, _, _ := rawRequest(t, p, "CONNECT "+addr+" HTTP/1.1\r\nHost: "+addr+"\r\n\r\n")

	// Require the tunnel to open to the name the allowlist knows
	want := []string{net.JoinHostPort(fixture.Host, strconv.Itoa(fixture.Port))}
	if status != http.StatusOK || !cmp.Equal(u.dials(), want) {
		t.Errorf("CONNECT %s = %d with dials %v, want %d and %v", addr, status, u.dials(), http.StatusOK, want)
	}
}

// A tunnel that names no port is turned away as malformed.
func TestProxyRefusesATunnelWithoutAPort(t *testing.T) {
	p, _, _ := proxying(t, listed(fixture.Port))

	status, _, _ := rawRequest(t, p, "CONNECT "+fixture.Host+" HTTP/1.1\r\nHost: "+fixture.Host+"\r\n\r\n")

	// Require the request to be called malformed
	if status != http.StatusBadRequest {
		t.Errorf("CONNECT %s = %d, want %d", fixture.Host, status, http.StatusBadRequest)
	}
}

// A listed host that cannot be reached is reported as a bad gateway.
func TestProxyReportsAnUnreachableHost(t *testing.T) {
	p, _, _ := proxying(t, listed(fixture.Port))
	addr := net.JoinHostPort(fixture.Host, strconv.Itoa(fixture.Port))

	status, _, _ := rawRequest(t, p, "CONNECT "+addr+" HTTP/1.1\r\nHost: "+addr+"\r\n\r\n")

	// Require the failure to name the gateway
	if status != http.StatusBadGateway {
		t.Errorf("CONNECT %s to nowhere = %d, want %d", addr, status, http.StatusBadGateway)
	}
}

// HTTP/2 runs inside a tunnel to a listed host.
func TestProxyCarriesHTTP2ThroughTheTunnel(t *testing.T) {
	p, u, _ := proxying(t, listed(fixture.Port))
	s := replying(t, true)
	u.route(fixture.Port, s)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+fixture.Host+"/", nil)

	resp, err := client(t, p, s).Do(req)

	// Require the answer to come over HTTP/2
	if err != nil {
		t.Fatalf("GET https://%s = %v, want an answer", fixture.Host, err)
	}
	_ = resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Errorf("GET https://%s came over %s, want HTTP/2", fixture.Host, resp.Proto)
	}
}

// Plain web traffic to a listed host is forwarded.
func TestProxyForwardsPlainTrafficToAListedHost(t *testing.T) {
	p, u, _ := proxying(t, listed(plainPort))
	s := replying(t, false)
	u.route(plainPort, s)

	status, body := get(t, client(t, p), "http://"+fixture.Host+"/")

	// Require the host's own answer
	if status != http.StatusOK || body != fixture.Reply {
		t.Errorf("GET http://%s = %d %q, want %d %q", fixture.Host, status, body, http.StatusOK, fixture.Reply)
	}
}

// Plain web traffic to a host the allowlist lacks is refused before the proxy reaches out to it.
func TestProxyRefusesPlainTrafficToAnUnlistedHost(t *testing.T) {
	p, u, _ := proxying(t, listed(plainPort))

	status, _ := get(t, client(t, p), "http://"+fixture.RefusedName+"/")

	// Require a refusal and no dial at all
	if status != http.StatusForbidden || len(u.dials()) != 0 {
		t.Errorf("GET http://%s = %d with dials %v, want %d and none", fixture.RefusedName, status, u.dials(), http.StatusForbidden)
	}
}

// A request for TLS that comes without a tunnel is turned away, since the proxy reaches TLS hosts through tunnels alone.
func TestProxyRefusesTLSWithoutATunnel(t *testing.T) {
	p, u, _ := proxying(t, listed(fixture.Port))

	status, _, _ := rawRequest(t, p, "GET https://"+fixture.Host+"/ HTTP/1.1\r\nHost: "+fixture.Host+"\r\n\r\n")

	// Require the request to be called malformed and no dial at all
	if status != http.StatusBadRequest || len(u.dials()) != 0 {
		t.Errorf("GET https://%s = %d with dials %v, want %d and none", fixture.Host, status, u.dials(), http.StatusBadRequest)
	}
}

// A request that names no host, as one meant for a server rather than a proxy, is turned away as malformed.
func TestProxyRefusesARequestWithoutAHost(t *testing.T) {
	p, u, _ := proxying(t, listed(plainPort))

	status, _, _ := rawRequest(t, p, "GET / HTTP/1.1\r\nHost: "+fixture.Host+"\r\n\r\n")

	// Require the request to be called malformed and no dial at all
	if status != http.StatusBadRequest || len(u.dials()) != 0 {
		t.Errorf("GET / = %d with dials %v, want %d and none", status, u.dials(), http.StatusBadRequest)
	}
}

// An upload that waits for leave to continue reaches a listed host whole.
func TestProxyCarriesAnUploadThatWaitsToContinue(t *testing.T) {
	p, u, _ := proxying(t, listed(plainPort))
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		_, _ = fmt.Fprint(w, n)
	}))
	t.Cleanup(s.Close)
	u.route(plainPort, s)
	c := client(t, p)
	c.Transport.(*http.Transport).ExpectContinueTimeout = 5 * time.Second
	body := bytes.Repeat([]byte(fixture.Message), 1<<16)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+fixture.Host+"/", bytes.NewReader(body))
	req.Header.Set("Expect", "100-continue")

	resp, err := c.Do(req)

	// Require the host to count every byte sent
	if err != nil {
		t.Fatalf("POST http://%s = %v, want an answer", fixture.Host, err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if want := strconv.Itoa(len(body)); string(got) != want {
		t.Errorf("host counted %s bytes, want %s", got, want)
	}
}

// A WebSocket to a listed host switches protocols and carries bytes both ways.
func TestProxyCarriesAWebSocket(t *testing.T) {
	p, u, _ := proxying(t, listed(plainPort))
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		c, rw, err := http.NewResponseController(w).Hijack()
		// Leave a connection that cannot be taken over
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = rw.Flush()
		b := make([]byte, len(fixture.Message))
		_, _ = io.ReadFull(rw, b)
		_, _ = c.Write([]byte(fixture.Reply))
	}))
	t.Cleanup(s.Close)
	u.route(plainPort, s)
	status, r, c := rawRequest(t, p, "GET http://"+fixture.Host+"/ HTTP/1.1\r\nHost: "+fixture.Host+
		"\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")

	_, _ = c.Write([]byte(fixture.Message))
	got := make([]byte, len(fixture.Reply))
	_, err := io.ReadFull(r, got)

	// Require the switch and the host's reply after it
	if status != http.StatusSwitchingProtocols || err != nil || string(got) != fixture.Reply {
		t.Errorf("WebSocket = %d, read %q, %v, want %d and %q", status, got, err, http.StatusSwitchingProtocols, fixture.Reply)
	}
}

// Requests to a listed host share one connection to it rather than opening one each.
func TestProxyReusesItsConnectionToAHost(t *testing.T) {
	p, u, _ := proxying(t, listed(plainPort))
	var opened atomic.Int32
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture.Reply)
	}))
	s.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		// Count each connection the proxy opens
		if st == http.StateNew {
			opened.Add(1)
		}
	}
	s.Start()
	t.Cleanup(s.Close)
	u.route(plainPort, s)
	c := client(t, p)

	// Fetch the host a few times
	for range 3 {
		_, _ = get(t, c, "http://"+fixture.Host+"/")
	}

	// Require one connection for every request
	if n := opened.Load(); n != 1 {
		t.Errorf("proxy opened %d connections to the host, want 1", n)
	}
}

// Every refusal leaves a log line that names the host and its port.
func TestProxyLogsARefusal(t *testing.T) {
	p, _, log := proxying(t, listed(fixture.Port))
	addr := net.JoinHostPort(fixture.RefusedName, strconv.Itoa(fixture.Port))

	_, _, _ = rawRequest(t, p, "CONNECT "+addr+" HTTP/1.1\r\nHost: "+addr+"\r\n\r\n")

	var line map[string]any
	// Require one line that names the refused host
	if err := json.Unmarshal(log.Bytes(), &line); err != nil {
		t.Fatalf("log %q = %v, want one JSON line", log.String(), err)
	}
	delete(line, "time")
	want := map[string]any{"level": "INFO", "msg": "refused", "host": fixture.RefusedName, "port": float64(fixture.Port)}
	if diff := cmp.Diff(want, line); diff != "" {
		t.Errorf("refusal log (-want +got):\n%s", diff)
	}
}

// Ending the context ends the cage's connection, even halfway through a request.
func TestHandleReturnsWhenTheContextEnds(t *testing.T) {
	p, _, _ := proxying(t)
	cage, host := net.Pipe()
	t.Cleanup(func() { _ = cage.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Handle(ctx, host)
	}()
	_ = cage.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(cage, "GET http://"+fixture.Host+"/ HTTP/1.1\r\n")

	cancel()

	// Require Handle to return soon after
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Handle still runs after its context ended")
	}
}

// A closed proxy turns the cage's connections away at once.
func TestHandleReturnsOnceTheProxyCloses(t *testing.T) {
	p, _, _ := proxying(t)
	_ = p.Close()
	cage, host := net.Pipe()
	t.Cleanup(func() { _ = cage.Close() })
	done := make(chan struct{})

	go func() {
		defer close(done)
		p.Handle(t.Context(), host)
	}()

	// Require Handle to return without serving
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Handle still runs on a closed proxy")
	}
}
