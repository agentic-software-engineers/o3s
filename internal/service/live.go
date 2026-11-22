package service

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/hansehart/o3s/internal/proxy"
)

// Live keeps the latest snapshot of the home's files, and reads them again whenever one of them changes.
type Live struct {
	files Files
	log   *slog.Logger
	cur   atomic.Pointer[Snapshot]
	seen  [2]os.FileInfo
}

// NewLive reads the home's files once and hands back the live view of them.
func NewLive(f Files, log *slog.Logger) *Live {
	l := &Live{files: f, log: log}
	l.seen = l.look()
	l.cur.Store(Load(f, log))
	return l
}

// Run looks at the files on every tick until the context ends, and takes a fresh snapshot once one of them changed.
func (l *Live) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	// Look at the files on every tick until the context ends
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := l.look()
		// Take a fresh snapshot once a file changed
		if changed(l.seen, now) {
			l.seen = now
			l.cur.Store(Load(l.files, l.log))
		}
	}
}

// look reads what each file is right now, nil for a file that is missing.
func (l *Live) look() [2]os.FileInfo {
	var out [2]os.FileInfo
	// Read each file's details
	for i, path := range []string{l.files.Config, l.files.Secrets} {
		out[i], _ = os.Stat(path)
	}
	return out
}

// changed reports whether any file appeared, went, was replaced, or was written since the last look.
func changed(before, now [2]os.FileInfo) bool {
	// Compare each file with its last look
	for i := range before {
		a, b := before[i], now[i]
		// Report a file that appeared or went
		if (a == nil) != (b == nil) {
			return true
		}
		// Report a file that was replaced or written
		if a != nil && (!os.SameFile(a, b) || !a.ModTime().Equal(b.ModTime())) {
			return true
		}
	}
	return false
}

// Allows reports whether the latest snapshot lets the host through on the port.
func (l *Live) Allows(host string, port int) bool {
	return l.cur.Load().Allows(host, port)
}

// Lists reports whether the latest snapshot holds the name.
func (l *Live) Lists(name string) bool {
	return l.cur.Load().Lists(name)
}

// Secret hands back the host's secret in the latest snapshot.
func (l *Live) Secret(host string) (proxy.Secret, bool) {
	return l.cur.Load().Secret(host)
}
