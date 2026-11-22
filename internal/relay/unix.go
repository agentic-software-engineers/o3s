//go:build unix

package relay

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// maxSocketPath is the longest socket path every Unix the host may run takes: 104 bytes on macOS and the BSDs, 108 on
// Linux, less the closing zero.
const maxSocketPath = 103

// socketMode lets the socket's owner and group connect and no one else.
const socketMode = 0o660

// defaultPoll is how often a listener checks that its socket is still in place when the transport names no interval.
const defaultPoll = 200 * time.Millisecond

var (
	// ErrSocketInUse reports a socket another listener still serves.
	ErrSocketInUse = errors.New("socket in use")
	// ErrPathTooLong reports a socket path longer than the kernel takes.
	ErrPathTooLong = errors.New("socket path too long")
)

// Unix is a transport over Unix sockets in one folder the host and the cage share, one socket for each service.
type Unix struct {
	// Dir is the folder that holds the sockets.
	Dir string
	// Poll is how often a listener checks that its socket is still in place.
	Poll time.Duration
}

// Path is where the service's socket lives.
func (u Unix) Path(s Service) string {
	return filepath.Join(u.Dir, string(s)+".sock")
}

// Listen makes the service's socket, taking the place of one a listener left behind, and watches it so the listener
// ends when the socket goes.
func (u Unix) Listen(s Service) (net.Listener, error) {
	// Turn away a service the relay does not offer
	if err := known(s); err != nil {
		return nil, err
	}
	p := u.Path(s)
	// Turn away a path the kernel would cut short
	if len(p) > maxSocketPath {
		return nil, fmt.Errorf("%w: %s", ErrPathTooLong, p)
	}
	// Clear a socket left behind, but leave one a listener still serves
	if err := clearStale(p); err != nil {
		return nil, err
	}
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", p)
	// Hand back a socket that cannot be made
	if err != nil {
		return nil, err
	}
	ul := l.(*net.UnixListener)
	ul.SetUnlinkOnClose(false)
	info, err := lockDown(p)
	// Leave no socket behind when it cannot be locked down
	if err != nil {
		_ = ul.Close()
		_ = os.Remove(p)
		return nil, err
	}
	poll := u.Poll
	// Fall back to the default interval when none is named
	if poll <= 0 {
		poll = defaultPoll
	}
	return watch(ul, p, info, poll), nil
}

// Dial connects to the service's socket, as the cage does.
func (u Unix) Dial(ctx context.Context, s Service) (net.Conn, error) {
	// Turn away a service the relay does not offer
	if err := known(s); err != nil {
		return nil, err
	}
	var d net.Dialer
	return d.DialContext(ctx, "unix", u.Path(s))
}

// clearStale removes a socket no listener answers on, and turns away one a listener still serves.
func clearStale(p string) error {
	// Go on when no socket is in the way
	if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	d := net.Dialer{Timeout: time.Second}
	c, err := d.DialContext(context.Background(), "unix", p)
	// Leave a socket that still answers to its listener
	if err == nil {
		_ = c.Close()
		return fmt.Errorf("%w: %s", ErrSocketInUse, p)
	}
	// Remove the socket nobody answers on
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// lockDown gives the socket its mode and reports what it is, so a watch can tell it from a later one.
func lockDown(p string) (fs.FileInfo, error) {
	// Hand back a mode that cannot be set
	if err := os.Chmod(p, socketMode); err != nil {
		return nil, err
	}
	return os.Lstat(p)
}

// watched is a listener that ends when its socket goes or another takes its place.
type watched struct {
	*net.UnixListener
	path string
	info fs.FileInfo
	done chan struct{}
	once sync.Once
}

// watch starts checking that the listener's socket stays in place.
func watch(l *net.UnixListener, p string, info fs.FileInfo, poll time.Duration) *watched {
	w := &watched{UnixListener: l, path: p, info: info, done: make(chan struct{})}
	go w.watch(poll)
	return w
}

// watch ends the listener once its socket is gone, checking at every tick until the listener closes.
func (w *watched) watch(poll time.Duration) {
	t := time.NewTicker(poll)
	defer t.Stop()
	// Check at every tick until the listener closes
	for {
		select {
		case <-w.done:
			return
		case <-t.C:
		}
		// End the listener once its socket is no longer the one it made
		if !w.inPlace() {
			_ = w.UnixListener.Close()
			return
		}
	}
}

// inPlace reports whether the socket the listener made is still at its path.
func (w *watched) inPlace() bool {
	info, err := os.Lstat(w.path)
	return err == nil && os.SameFile(info, w.info)
}

// Close ends the listener and its watch, and removes its socket unless another has taken its place.
func (w *watched) Close() error {
	w.once.Do(func() { close(w.done) })
	err := w.UnixListener.Close()
	// Remove the socket only while it is still this listener's own
	if w.inPlace() {
		_ = os.Remove(w.path)
	}
	return err
}
