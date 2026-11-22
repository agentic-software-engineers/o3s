// Package service serves the cage's connections from a home's own files: it keeps a live view of the allowlist and
// the secrets, and runs the proxy and the DNS behind the relay with it.
package service

import (
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/hansehart/o3s/internal/config"
	"github.com/hansehart/o3s/internal/proxy"
	"github.com/hansehart/o3s/internal/secrets"
)

// Files names the home's files the service reads.
type Files struct {
	// Config is the home's allowlist, o3s.json in the home's folder.
	Config string
	// Secrets is the file that holds the real values of the secrets.
	Secrets string
}

// Snapshot is one consistent read of the allowlist and the secrets.
type Snapshot struct {
	hosts []config.Host
	store secrets.Store
}

// Load reads the allowlist and the secrets. An allowlist that fails to read leaves every host refused, and a secrets
// file that fails to read leaves every secret empty, each with a log line saying why.
func Load(f Files, log *slog.Logger) *Snapshot {
	s := &Snapshot{}
	c, err := config.Load(filepath.Dir(f.Config))
	// Refuse every host while the allowlist fails to read
	if err != nil {
		log.Error("allowlist unreadable, refusing every host", "err", err)
		return s
	}
	s.hosts = c.Hosts
	store, err := secrets.Load(f.Secrets)
	// Send every secret empty while the secrets fail to read
	if err != nil {
		log.Error("secrets unreadable, sending every secret empty", "err", err)
	}
	s.store = store
	return s
}

// open hands back the allowlist with the hosts whose end time has come left out.
func (s *Snapshot) open(now time.Time) *config.Config {
	c := &config.Config{}
	// Keep each host that is still open
	for _, h := range s.hosts {
		// Keep a host whose end time is still ahead, or that stays open for good
		if !h.Expired(now) {
			c.Hosts = append(c.Hosts, h)
		}
	}
	return c
}

// Allows reports whether an open host of the allowlist lets the host through on the port.
func (s *Snapshot) Allows(host string, port int) bool {
	return s.open(time.Now()).Allows(host, port)
}

// Lists reports whether an open host of the allowlist holds the name.
func (s *Snapshot) Lists(name string) bool {
	return s.open(time.Now()).Lists(name)
}

// Secret hands back the secret of the open entry that names the host, its own first and then the nearest wildcard
// above it, with the sites the entry names, or the header while it leaves them empty.
func (s *Snapshot) Secret(host string) (proxy.Secret, bool) {
	c := s.open(time.Now())
	h, ok := c.Find(host)
	// Look for the nearest wildcard where the host lacks an entry of its own
	for i := strings.IndexByte(host, '.'); !ok && i >= 0; i = nextDot(host, i) {
		h, ok = c.Find("*" + host[i:])
	}
	// Carry a host sealed while its entry lacks a secret
	if !ok || h.Secret == "" {
		return proxy.Secret{}, false
	}
	value, _ := s.store.Value(h.Secret)
	sites := h.Inject
	// Swap the secret into the header while the entry leaves its sites empty
	if len(sites) == 0 {
		sites = []string{proxy.Header}
	}
	return proxy.Secret{Value: value, Sites: sites}, true
}

// nextDot hands back the place of the dot after the one at i, or -1 past the last.
func nextDot(host string, i int) int {
	j := strings.IndexByte(host[i+1:], '.')
	// Report the end once the name's last dot is passed
	if j < 0 {
		return -1
	}
	return i + 1 + j
}
