package proxy

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"sync"
	"time"
)

// A host's certificate starts two days back, so a cage whose clock runs behind takes it, lasts about half a year, well
// inside what strict clients accept, and is signed afresh a day before it ends.
const (
	certBackdate = 48 * time.Hour
	certLife     = 199 * 24 * time.Hour
	certRenew    = 24 * time.Hour
)

// handshakeTimeout bounds the cage's TLS handshake with the proxy.
const handshakeTimeout = 30 * time.Second

// maxCommonName is the longest common name a certificate may carry.
const maxCommonName = 64

// errNoCA reports a CA that lacks its certificate.
var errNoCA = errors.New("no CA to sign with")

// secretOf hands back the secret the host carries, where the proxy can open the host's traffic.
func (p *Proxy) secretOf(host string) (Secret, bool) {
	// Keep the host sealed while the proxy lacks secrets or a CA
	if p.secrets == nil || p.certs == nil {
		return Secret{}, false
	}
	return p.secrets(host)
}

// intercept ends the cage's TLS at the proxy with a certificate for the host, swaps the secret into every request the
// cage sends inside, and sends it on to the host over TLS the proxy checks.
func (p *Proxy) intercept(w http.ResponseWriter, host string, port int, sec Secret) {
	c, rw, err := http.NewResponseController(w).Hijack()
	// Give up when the server keeps the connection
	if err != nil {
		return
	}
	defer c.Close()
	_, _ = rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
	// Stop when the cage is gone before the tunnel opens
	if err := rw.Flush(); err != nil {
		return
	}
	tc := tls.Server(&buffered{Conn: c, r: rw.Reader}, &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return p.certs.forHost(host) },
		NextProtos:     []string{"h2", "http/1.1"},
		MinVersion:     tls.VersionTLS12,
	})
	ctx, cancel := context.WithTimeout(p.ctx, handshakeTimeout)
	err = tc.HandshakeContext(ctx)
	cancel()
	// Stop when the TLS handshake fails
	if err != nil {
		p.log.Warn("handshake", "host", host, "err", err)
		return
	}
	done := make(chan struct{})
	var once sync.Once
	srv := &http.Server{
		Handler:           p.injector(host, port, sec),
		ReadHeaderTimeout: headerTimeout,
		BaseContext:       func(net.Listener) context.Context { return p.ctx },
		ErrorLog:          p.srv.ErrorLog,
		ConnState: func(_ net.Conn, st http.ConnState) {
			// Tell the tunnel once the server is done with the cage's connection
			if st == http.StateClosed || st == http.StateHijacked {
				once.Do(func() { close(done) })
			}
		},
	}
	l := &single{c: tc, done: make(chan struct{})}
	go func() { _ = srv.Serve(l) }()
	stop := context.AfterFunc(p.ctx, func() { _ = srv.Close() })
	defer stop()
	<-done
	_ = l.Close()
}

// injector serves the requests inside one host's tunnel: it swaps the secret in and masks it in what comes back.
func (p *Proxy) injector(host string, port int, sec Secret) http.Handler {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	forms := secretForms(sec.Value)
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "https"
			pr.Out.URL.Host = addr
			pr.Out.Host = pr.In.Host
			// Let the proxy pick the compression, so it reads every answer it masks
			pr.Out.Header.Del("Accept-Encoding")
		},
		Transport:      p.secure,
		ModifyResponse: func(resp *http.Response) error { return p.mask(resp, host, forms) },
		ErrorHandler:   p.unreachable,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		named, _, err := target(r.Host, port)
		// Turn away a request for another host, keeping the secret with the host it belongs to
		if err != nil || named != host {
			p.log.Info("misdirected", "host", host, "asked", r.Host)
			http.Error(w, "o3s carries this tunnel to "+host+" alone", http.StatusMisdirectedRequest)
			return
		}
		// Turn away a request too large to carry the secret
		if err := swap(r, p.marker, sec); err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		rp.ServeHTTP(w, r)
	})
}

// certs signs a certificate for each host the proxy opens, and keeps it until it nears its end.
type certs struct {
	ca  *tls.Certificate
	key *ecdsa.PrivateKey

	mu    sync.Mutex
	hosts map[string]*tls.Certificate
	err   error
}

// newCerts makes the signer for the CA, with one key every host's certificate shares.
func newCerts(ca *tls.Certificate) *certs {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return &certs{ca: ca, key: key, hosts: map[string]*tls.Certificate{}, err: err}
}

// forHost hands back the host's certificate, signing a new one where none is kept or the kept one nears its end.
func (cs *certs) forHost(host string) (*tls.Certificate, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// Hand back the failure to make the shared key
	if cs.err != nil {
		return nil, cs.err
	}
	// Hand back the kept certificate while it lasts
	if c, ok := cs.hosts[host]; ok && time.Until(c.Leaf.NotAfter) > certRenew {
		return c, nil
	}
	c, err := cs.sign(host)
	// Hand back a certificate that fails to sign
	if err != nil {
		return nil, err
	}
	cs.hosts[host] = c
	return c, nil
}

// sign makes the host's certificate: named for it, for servers alone, short-lived, and tied to the CA by its key.
func (cs *certs) sign(host string) (*tls.Certificate, error) {
	issuer := cs.ca.Leaf
	// Read the CA's certificate where it came unparsed
	if issuer == nil {
		// Turn away a CA that lacks its certificate
		if len(cs.ca.Certificate) == 0 {
			return nil, errNoCA
		}
		var err error
		issuer, err = x509.ParseCertificate(cs.ca.Certificate[0])
		// Hand back a CA certificate that fails to parse
		if err != nil {
			return nil, err
		}
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	// Hand back a serial that fails to draw
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		NotBefore:    now.Add(-certBackdate),
		NotAfter:     now.Add(-certBackdate + certLife),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	// Name the host by its address where it is one, and by its name otherwise
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}
	// Carry the host as common name too where it fits, for clients that still read it
	if len(host) < maxCommonName {
		tmpl.Subject = pkix.Name{CommonName: host}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer, &cs.key.PublicKey, cs.ca.PrivateKey)
	// Hand back a certificate that fails to sign
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	// Hand back a certificate that fails to parse
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der, issuer.Raw}, PrivateKey: cs.key, Leaf: leaf}, nil
}

// buffered is a connection whose first bytes wait in a reader already.
type buffered struct {
	net.Conn
	r *bufio.Reader
}

// Read reads what the reader holds first, then the connection.
func (b *buffered) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

// single is a listener that hands over one connection and then waits until it closes.
type single struct {
	c    net.Conn
	once sync.Once
	done chan struct{}
	shut sync.Once
}

// Accept hands over the connection the first time and waits for the close after.
func (l *single) Accept() (net.Conn, error) {
	var c net.Conn
	l.once.Do(func() { c = l.c })
	// Hand over the connection the first time
	if c != nil {
		return c, nil
	}
	<-l.done
	return nil, net.ErrClosed
}

// Close ends the wait.
func (l *single) Close() error {
	l.shut.Do(func() { close(l.done) })
	return nil
}

// Addr names the connection's own address.
func (l *single) Addr() net.Addr {
	return l.c.LocalAddr()
}
