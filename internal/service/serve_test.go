//go:build unix

package service_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/hansehart/o3s/internal/certtest"
	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/proxy"
	"github.com/hansehart/o3s/internal/relay"
	"github.com/hansehart/o3s/internal/service"
)

// resolver answers every name with the fixture's address.
type resolver struct{}

// LookupNetIP answers with the fixture's address.
func (resolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr(fixture.Address)}, nil
}

// nameservers is an empty upstream list.
type nameservers struct{}

// Nameservers hands back an empty list.
func (nameservers) Nameservers() ([]string, error) { return nil, nil }

// socketDir hands a test a short folder for sockets, since a socket's path has to fit the kernel's small limit.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "o3s") //nolint:usetesting // t.TempDir's long path outgrows a socket's path limit
	// Stop when the folder fails to appear
	if err != nil {
		t.Fatalf("Setup: MkdirTemp = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// serving runs the service over the home's files until the test ends, the host's traffic landing at the server.
func serving(t *testing.T, h aHome, upstream *httptest.Server) (relay.Unix, *x509.CertPool) {
	t.Helper()
	ca, roots := certtest.CA(t)
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	_ = os.WriteFile(caFile, certtest.PEM(t, ca), 0o600)
	upstreamRoots := x509.NewCertPool()
	upstreamRoots.AddCert(upstream.Certificate())
	dir := socketDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.Serve(ctx, service.Options{
			Files:     h.files,
			CA:        caFile,
			Marker:    fixture.Marker,
			Transport: relay.Unix{Dir: dir},
			Log:       h.logger(),
			Poll:      livePoll,
			Resolver:  resolver{},
			Roots:     upstreamRoots,
			Upstream:  nameservers{},
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, upstream.Listener.Addr().String())
			},
		})
	}()
	t.Cleanup(func() {
		cancel()
		// Report a service that fails to stop cleanly
		if err := <-done; err != nil {
			t.Errorf("Serve = %v, want a clean stop", err)
		}
	})
	return relay.Unix{Dir: dir}, roots
}

// dialing reaches the service's socket once the service listens on it.
func dialing(t *testing.T, u relay.Unix, s relay.Service) net.Conn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	// Try again until the service listens or the time runs out
	for {
		c, err := u.Dial(t.Context(), s)
		// Hand back the first connection that gets through
		if err == nil {
			t.Cleanup(func() { _ = c.Close() })
			return c
		}
		// Stop once the service has had time enough to listen
		if time.Now().After(deadline) {
			t.Fatalf("Setup: Dial(%s) = %v", s, err)
		}
		time.Sleep(livePoll)
	}
}

// The cage's request through the proxy socket reaches its host with the host's secret in place of the marker.
func TestServeSwapsTheSecretForTheCagesRequest(t *testing.T) {
	got := make(chan string, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Authorization")
	}))
	t.Cleanup(upstream.Close)
	h := homeWith(t, []config.Change{carrying(fixture.Host, fixture.Secret, proxy.Header)}, fixture.Secret+"="+fixture.TokenValue)
	u, roots := serving(t, h, upstream)
	_ = dialing(t, u, relay.Proxy).Close()
	tr := &http.Transport{
		Proxy:           http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.invalid"}),
		DialContext:     func(ctx context.Context, _, _ string) (net.Conn, error) { return u.Dial(ctx, relay.Proxy) },
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
	}
	t.Cleanup(tr.CloseIdleConnections)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+fixture.Host+"/", nil)
	req.Header.Set("Authorization", "Bearer "+fixture.Marker)

	resp, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Do(req)

	// Require the host to read its own secret
	if err != nil {
		t.Fatalf("GET https://%s = %v, want an answer", fixture.Host, err)
	}
	_ = resp.Body.Close()
	if a := <-got; a != "Bearer "+fixture.TokenValue {
		t.Errorf("host read Authorization %q, want %q", a, "Bearer "+fixture.TokenValue)
	}
}

// The cage's lookup of a name the allowlist lacks, through the DNS socket, comes back as missing.
func TestServeAnswersTheCagesLookups(t *testing.T) {
	upstream := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(upstream.Close)
	h := homeWith(t, []config.Change{open(fixture.Host)})
	u, _ := serving(t, h, upstream)
	c := dialing(t, u, relay.DNS)
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 1, RecursionDesired: true})
	_ = b.StartQuestions()
	_ = b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName(fixture.RefusedName + "."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET})
	msg, _ := b.Finish()

	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(min(len(msg), math.MaxUint16))), msg...))
	var size [2]byte
	_, err := io.ReadFull(c, size[:])
	answer := make([]byte, binary.BigEndian.Uint16(size[:]))
	_, _ = io.ReadFull(c, answer)

	var m dnsmessage.Message
	// Require the refused name answered as missing
	if err != nil || m.Unpack(answer) != nil || m.RCode != dnsmessage.RCodeNameError {
		t.Errorf("lookup of %s = %v, %v, want NXDOMAIN", fixture.RefusedName, m.RCode, err)
	}
}
