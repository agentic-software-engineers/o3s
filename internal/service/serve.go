package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/hansehart/o3s/internal/dns"
	"github.com/hansehart/o3s/internal/proxy"
	"github.com/hansehart/o3s/internal/relay"
)

// defaultPoll is how often the service looks for changed files where its options leave the interval at zero.
const defaultPoll = time.Second

// Options are what the service serves the cage with.
type Options struct {
	// Files are the home's allowlist and secrets.
	Files Files
	// CA is the file that holds the proxy's CA, its key and then its cert.
	CA string
	// Marker is the placeholder the cage sends in place of every secret.
	Marker string
	// Transport carries the cage's connections to the service.
	Transport relay.Transport
	// Log takes the service's log lines.
	Log *slog.Logger
	// Poll is how often the service looks for changed files.
	Poll time.Duration
	// Resolver finds the addresses of a listed name, and the system's does where it is empty.
	Resolver proxy.Resolver
	// Roots are the CAs the proxy trusts a host's certificate by, and the system's where it is empty.
	Roots *x509.CertPool
	// Upstream lists the nameservers, and the host's own do where it is empty.
	Upstream dns.Upstream
	// Dial reaches an address the proxy checked, and a plain dialer does where it is empty.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Serve runs the proxy and the DNS behind the relay with a live view of the home's files, until the context ends.
func Serve(ctx context.Context, o Options) error {
	// Discard the log lines while the options leave the log empty
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	// Look for changed files every second while the options leave the interval at zero
	if o.Poll <= 0 {
		o.Poll = defaultPoll
	}
	// Ask the host's own nameservers while the options leave the upstream empty
	if o.Upstream == nil {
		o.Upstream = dns.System()
	}
	live := NewLive(o.Files, o.Log)
	go live.Run(ctx, o.Poll)
	ca, err := loadCA(o.CA)
	// Carry every host sealed while the CA fails to read
	if err != nil {
		o.Log.Error("CA unreadable, carrying every host sealed", "err", err)
	}
	p := proxy.New(proxy.Options{
		Policy: live, Dial: o.Dial, Resolver: o.Resolver, Log: o.Log,
		CA: ca, Roots: o.Roots, Marker: o.Marker, Secrets: live.Secret,
	})
	defer p.Close()
	d := dns.New(dns.Options{Policy: live, Upstream: o.Upstream, Log: o.Log})
	r := relay.New(o.Transport)
	_ = r.Handle(relay.Proxy, p.Handle)
	_ = r.Handle(relay.DNS, d.Handle)
	return r.Serve(ctx)
}

// loadCA reads the CA's key pair from its file.
func loadCA(path string) (*tls.Certificate, error) {
	data, err := os.ReadFile(path)
	// Hand back a file that fails to read
	if err != nil {
		return nil, err
	}
	ca, err := tls.X509KeyPair(data, data)
	// Hand back a key pair that fails to load
	if err != nil {
		return nil, err
	}
	return &ca, nil
}
