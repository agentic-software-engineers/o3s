package proxy_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"log/slog"
	"maps"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/proxy"
)

// strictValidity is the longest a server certificate may last for the strictest clients.
const strictValidity = 397 * 24 * time.Hour

// testCA makes a CA shaped like the one o3s keeps for a home, and a pool that trusts it.
func testCA(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	// Stop when the key fails to generate
	if err != nil {
		t.Fatalf("Setup: GenerateKey = %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"o3s"}, CommonName: "Egress Proxy CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	// Stop when the CA fails to sign
	if err != nil {
		t.Fatalf("Setup: CreateCertificate = %v", err)
	}
	leaf, _ := x509.ParseCertificate(der)
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, roots
}

// received is what a host read from one request.
type received struct {
	Header http.Header
	Query  string
	Body   string
}

// recording starts a TLS test server that hands every request it reads to the test, and answers with the handler.
func recording(t *testing.T, answer http.HandlerFunc) (*httptest.Server, <-chan received) {
	t.Helper()
	got := make(chan received, 8)
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- received{Header: r.Header.Clone(), Query: r.URL.RawQuery, Body: string(b)}
		answer(w, r)
	}))
	s.EnableHTTP2 = true
	s.StartTLS()
	t.Cleanup(s.Close)
	return s, got
}

// silent is a handler that answers with its status alone.
func silent(http.ResponseWriter, *http.Request) {}

// intercept is a proxy whose listed host carries the token in the sites, with its host behind it.
type intercept struct {
	proxy  *proxy.Proxy
	up     *upstreams
	roots  *x509.CertPool
	ca     *x509.Certificate
	client *http.Client
	got    <-chan received
}

// intercepting starts a proxy that holds the token for the listed host in the sites, trusting the host's test server,
// and a client in the cage that trusts the proxy's CA.
func intercepting(t *testing.T, value string, sites []string, answer http.HandlerFunc) intercept {
	t.Helper()
	ca, roots := testCA(t)
	s, got := recording(t, answer)
	upstreamRoots := x509.NewCertPool()
	upstreamRoots.AddCert(s.Certificate())
	u := &upstreams{routes: map[string]string{}}
	u.route(fixture.Port, s)
	p := proxy.New(proxy.Options{
		Policy: &config.Config{Hosts: []config.Host{listed(fixture.Port)}},
		Dial:   u.dial,
		Log:    slog.New(slog.DiscardHandler),
		CA:     &ca,
		Roots:  upstreamRoots,
		Marker: fixture.Marker,
		Secrets: func(host string) (proxy.Secret, bool) {
			return proxy.Secret{Value: value, Sites: sites}, host == fixture.Host
		},
	})
	t.Cleanup(func() { _ = p.Close() })
	c := client(t, p)
	c.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots
	return intercept{proxy: p, up: u, roots: roots, ca: ca.Leaf, client: c, got: got}
}

// send makes a request through the proxy and hands back the answer's status, headers and body.
func send(t *testing.T, c *http.Client, method, addr string, header http.Header, body string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, addr, strings.NewReader(body))
	// Stop when the request fails to build
	if err != nil {
		t.Fatalf("Setup: NewRequest(%s) = %v", addr, err)
	}
	maps.Copy(req.Header, header)
	resp, err := c.Do(req)
	// Stop when the request fails
	if err != nil {
		t.Fatalf("%s %s = %v, want an answer", method, addr, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

// first hands back the first request the host read, failing the test once the wait runs out.
func first(t *testing.T, got <-chan received) received {
	t.Helper()
	// Wait for the host's record, or give up
	select {
	case r := <-got:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the host read no request")
		return received{}
	}
}

// hostURL is the listed host's address over TLS, with the path and query.
func hostURL(pathAndQuery string) string {
	return "https://" + fixture.Host + pathAndQuery
}

// The marker in a header reaches the host as the host's own secret.
func TestProxySwapsTheMarkerInAHeader(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, silent)

	_, _, _ = send(t, in.client, http.MethodGet, hostURL("/"), http.Header{"Authorization": {"Bearer " + fixture.Marker}}, "")

	// Require the host to read its secret where the marker was
	if got, want := first(t, in.got).Header.Get("Authorization"), "Bearer "+fixture.TokenValue; got != want {
		t.Errorf("host read Authorization %q, want %q", got, want)
	}
}

// The marker inside a Basic credential reaches the host as the host's own secret, encoded as the host expects.
func TestProxySwapsTheMarkerInsideABasicCredential(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, silent)
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(fixture.Repo+":"+fixture.Marker))

	_, _, _ = send(t, in.client, http.MethodGet, hostURL("/"), http.Header{"Authorization": {basic}}, "")

	// Require the host to decode its secret where the marker was
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(fixture.Repo+":"+fixture.TokenValue))
	if got := first(t, in.got).Header.Get("Authorization"); got != want {
		t.Errorf("host read Authorization %q, want %q", got, want)
	}
}

// The marker in the query reaches the host as its secret where the host carries it there.
func TestProxySwapsTheMarkerInTheQuery(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Query}, silent)

	_, _, _ = send(t, in.client, http.MethodGet, hostURL("/?key="+fixture.Marker), nil, "")

	// Require the host to read its secret in the query
	if got, want := first(t, in.got).Query, "key="+url.QueryEscape(fixture.TokenValue); got != want {
		t.Errorf("host read query %q, want %q", got, want)
	}
}

// The marker in the body reaches the host as its secret where the host carries it there.
func TestProxySwapsTheMarkerInTheBody(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Body}, silent)

	_, _, _ = send(t, in.client, http.MethodPost, hostURL("/"), nil, "token="+fixture.Marker)

	// Require the host to read its secret in the body
	if got, want := first(t, in.got).Body, "token="+fixture.TokenValue; got != want {
		t.Errorf("host read body %q, want %q", got, want)
	}
}

// The marker outside the sites the host carries its secret in stays the marker.
func TestProxyLeavesTheMarkerOutsideTheHostsSites(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, silent)

	_, _, _ = send(t, in.client, http.MethodPost, hostURL("/?key="+fixture.Marker), nil, fixture.Marker)

	// Require query and body to keep the marker
	got := first(t, in.got)
	want := received{Header: got.Header, Query: "key=" + fixture.Marker, Body: fixture.Marker}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("host read (-want +got):\n%s", diff)
	}
}

// A host free of secrets gets the marker as the cage sent it, through a sealed tunnel.
func TestProxyKeepsTheSecretFromAHostWithoutOne(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, silent)
	other, got := recording(t, silent)
	in.up.routeHost(fixture.Subdomain, fixture.Port, other)
	p := proxy.New(proxy.Options{
		Policy: &config.Config{Hosts: []config.Host{listed(fixture.Port), {Name: fixture.Subdomain, Ports: []int{fixture.Port}}}},
		Dial:   in.up.dial,
		Marker: fixture.Marker,
		Secrets: func(host string) (proxy.Secret, bool) {
			return proxy.Secret{Value: fixture.TokenValue, Sites: []string{proxy.Header}}, host == fixture.Host
		},
	})
	t.Cleanup(func() { _ = p.Close() })

	_, _, _ = send(t, client(t, p, other), http.MethodGet, "https://"+fixture.Subdomain+"/", http.Header{"Authorization": {"Bearer " + fixture.Marker}}, "")

	// Require the other host to read the marker alone
	if got, want := first(t, got).Header.Get("Authorization"), "Bearer "+fixture.Marker; got != want {
		t.Errorf("other host read Authorization %q, want %q", got, want)
	}
}

// A request inside a host's tunnel that names another host is refused, so a secret stays with its own host when hosts
// share the address.
func TestProxyRefusesAnotherHostInsideATunnel(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, silent)
	tlsConn := tunnelTo(t, in, fixture.Host)

	_, _ = io.WriteString(tlsConn, "GET / HTTP/1.1\r\nHost: "+fixture.OtherHost+"\r\nAuthorization: Bearer "+fixture.Marker+"\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(tlsConn), nil)

	// Require the request to be called misdirected, and the host's record to stay empty
	if err != nil {
		t.Fatalf("read the proxy's answer = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest || len(in.got) != 0 {
		t.Errorf("request for %s inside %s's tunnel = %d with %d reaching the host, want %d and none",
			fixture.OtherHost, fixture.Host, resp.StatusCode, len(in.got), http.StatusMisdirectedRequest)
	}
}

// tunnelTo opens a tunnel to the host through the proxy and speaks TLS inside it, trusting the proxy's CA.
func tunnelTo(t *testing.T, in intercept, host string) *tls.Conn {
	t.Helper()
	addr := net.JoinHostPort(host, strconv.Itoa(fixture.Port))
	status, r, c := rawRequest(t, in.proxy, "CONNECT "+addr+" HTTP/1.1\r\nHost: "+addr+"\r\n\r\n")
	// Stop when the tunnel stays shut
	if status != http.StatusOK || r.Buffered() != 0 {
		t.Fatalf("Setup: CONNECT %s = %d with %d bytes early, want %d", addr, status, r.Buffered(), http.StatusOK)
	}
	tlsConn := tls.Client(c, &tls.Config{ServerName: host, RootCAs: in.roots, MinVersion: tls.VersionTLS12})
	// Stop when the handshake fails
	if err := tlsConn.HandshakeContext(t.Context()); err != nil {
		t.Fatalf("Setup: TLS handshake with %s = %v", host, err)
	}
	return tlsConn
}

// The secret a host echoes back reaches the cage masked, in headers and body alike.
func TestProxyMasksTheSecretInAResponse(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Echo", fixture.TokenValue)
		_, _ = io.WriteString(w, "token "+fixture.TokenValue)
	})

	_, header, body := send(t, in.client, http.MethodGet, hostURL("/"), nil, "")

	// Require the secret nowhere, and the filler of its length in its place
	stars := strings.Repeat("*", len(fixture.TokenValue))
	got := []string{header.Get("X-Echo"), body}
	if diff := cmp.Diff([]string{stars, "token " + stars}, got); diff != "" {
		t.Errorf("cage read (-want +got):\n%s", diff)
	}
}

// The secret a host echoes inside base64 reaches the cage masked.
func TestProxyMasksTheSecretInsideBase64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(fixture.Repo + ":" + fixture.TokenValue))
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, encoded)
	})

	_, _, body := send(t, in.client, http.MethodGet, hostURL("/"), nil, "")

	// Require the decoded body to hold filler where the secret was
	decoded, _ := base64.StdEncoding.DecodeString(body)
	if len(body) != len(encoded) || bytes.Contains(decoded, []byte(fixture.TokenValue)) {
		t.Errorf("cage read %q, decoding to %q, want the secret masked at the same length", body, decoded)
	}
}

// The secret a host sends in pieces reaches the cage masked all the same.
func TestProxyMasksASecretSplitAcrossWrites(t *testing.T) {
	half := len(fixture.TokenValue) / 2
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, func(w http.ResponseWriter, _ *http.Request) {
		rc := http.NewResponseController(w)
		_, _ = io.WriteString(w, fixture.TokenValue[:half])
		_ = rc.Flush()
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(w, fixture.TokenValue[half:])
	})

	_, _, body := send(t, in.client, http.MethodGet, hostURL("/"), nil, "")

	// Require the whole secret masked
	if want := strings.Repeat("*", len(fixture.TokenValue)); body != want {
		t.Errorf("cage read %q, want %q", body, want)
	}
}

// The secret a host echoes inside a compressed body reaches the cage masked.
func TestProxyMasksTheSecretInACompressedResponse(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, fixture.TokenValue)
		_ = zw.Close()
	})

	_, _, body := send(t, in.client, http.MethodGet, hostURL("/"), nil, "")

	// Require the whole secret masked
	if want := strings.Repeat("*", len(fixture.TokenValue)); body != want {
		t.Errorf("cage read %q, want %q", body, want)
	}
}

// A secret still waiting for its value reaches the host empty, so the marker stays inside the proxy.
func TestProxySendsNothingForASecretWithoutAValue(t *testing.T) {
	in := intercepting(t, "", []string{proxy.Header}, silent)

	_, _, _ = send(t, in.client, http.MethodGet, hostURL("/"), http.Header{"Authorization": {"Bearer " + fixture.Marker}}, "")

	// Require the host to read the scheme alone
	if got := first(t, in.got).Header.Get("Authorization"); got != "Bearer" && got != "Bearer " {
		t.Errorf("host read Authorization %q, want it empty after the scheme", got)
	}
}

// HTTP/2 runs between the cage and the proxy inside a tunnel the proxy opens.
func TestProxySpeaksHTTP2InsideItsTunnel(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, silent)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, hostURL("/"), nil)

	resp, err := in.client.Do(req)

	// Require the answer to come over HTTP/2
	if err != nil {
		t.Fatalf("GET %s = %v, want an answer", hostURL("/"), err)
	}
	_ = resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Errorf("GET %s came over %s, want HTTP/2", hostURL("/"), resp.Proto)
	}
}

// The certificate the proxy shows for a host is one strict clients accept: named for the host, for servers alone,
// short-lived, and chained to the CA by its key.
func TestProxyShowsACertificateStrictClientsAccept(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, silent)

	leaf := tunnelTo(t, in, fixture.Host).ConnectionState().PeerCertificates[0]

	_, verr := leaf.Verify(x509.VerifyOptions{DNSName: fixture.Host, Roots: in.roots})
	type facts struct {
		Names     []string
		Usage     []x509.ExtKeyUsage
		Short     bool
		Verifies  bool
		OwnKeyIDs bool
	}
	got := facts{
		Names:     leaf.DNSNames,
		Usage:     leaf.ExtKeyUsage,
		Short:     leaf.NotAfter.Sub(leaf.NotBefore) <= strictValidity && leaf.NotBefore.Before(time.Now()),
		Verifies:  verr == nil,
		OwnKeyIDs: bytes.Equal(leaf.AuthorityKeyId, in.ca.SubjectKeyId) && !bytes.Equal(leaf.SubjectKeyId, in.ca.SubjectKeyId),
	}
	want := facts{Names: []string{fixture.Host}, Usage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, Short: true, Verifies: true, OwnKeyIDs: true}
	// Require every trait a strict client checks
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("certificate for %s (-want +got):\n%s", fixture.Host, diff)
	}
}

// A host whose certificate fails the proxy's check is left unreached, and the cage hears of a bad gateway.
func TestProxyRefusesAHostItCannotTrust(t *testing.T) {
	ca, roots := testCA(t)
	s, got := recording(t, silent)
	u := &upstreams{routes: map[string]string{}}
	u.route(fixture.Port, s)
	p := proxy.New(proxy.Options{
		Policy: &config.Config{Hosts: []config.Host{listed(fixture.Port)}},
		Dial:   u.dial,
		CA:     &ca,
		Roots:  x509.NewCertPool(),
		Marker: fixture.Marker,
		Secrets: func(string) (proxy.Secret, bool) {
			return proxy.Secret{Value: fixture.TokenValue, Sites: []string{proxy.Header}}, true
		},
	})
	t.Cleanup(func() { _ = p.Close() })
	c := client(t, p)
	c.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots

	status, _, _ := send(t, c, http.MethodGet, hostURL("/"), http.Header{"Authorization": {"Bearer " + fixture.Marker}}, "")

	// Require a bad gateway and an empty record at the host
	if status != http.StatusBadGateway || len(got) != 0 {
		t.Errorf("GET %s to an untrusted host = %d with %d reaching it, want %d and none", hostURL("/"), status, len(got), http.StatusBadGateway)
	}
}

// An answer in an encoding beyond the proxy's reach is withheld from the cage, since the secret could hide in it.
func TestProxyWithholdsAnAnswerItCannotRead(t *testing.T) {
	in := intercepting(t, fixture.TokenValue, []string{proxy.Header}, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		_, _ = io.WriteString(w, fixture.TokenValue)
	})

	status, _, body := send(t, in.client, http.MethodGet, hostURL("/"), http.Header{"Accept-Encoding": {"br"}}, "")

	// Require a bad gateway and a body free of the secret
	if status != http.StatusBadGateway || strings.Contains(body, fixture.TokenValue) {
		t.Errorf("GET %s answered in br = %d %q, want %d without the secret", hostURL("/"), status, body, http.StatusBadGateway)
	}
}
