//go:build unix

package relay_test

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/relay"
)

// socketDir hands a test a short folder for sockets, since a socket's path has to fit the kernel's small limit.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "o3s") //nolint:usetesting // t.TempDir's long path outgrows a socket's path limit
	// Stop when no folder can be made
	if err != nil {
		t.Fatalf("Setup: MkdirTemp = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// newUnixTransport hands a test a Unix socket transport in a folder of its own.
func newUnixTransport(t *testing.T) relay.Transport {
	t.Helper()
	return relay.Unix{Dir: socketDir(t)}
}

// The Unix socket transport keeps the transport contract.
func TestUnixTransportHoldsTheContract(t *testing.T) {
	holdsTransportContract(t, newUnixTransport)
}

// Each service listens on a socket named after it in the transport's folder.
func TestUnixPathNamesTheService(t *testing.T) {
	u := relay.Unix{Dir: fixture.SocketDir}

	got := u.Path(relay.Proxy)

	// Require the socket to carry the service's name
	if want := filepath.Join(fixture.SocketDir, "proxy.sock"); got != want {
		t.Errorf("Path(%s) = %q, want %q", relay.Proxy, got, want)
	}
}

// A socket left behind by a listener that is gone gives way to a new listener.
func TestUnixListenRemovesAStaleSocket(t *testing.T) {
	u := relay.Unix{Dir: socketDir(t)}
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: u.Path(relay.DNS), Net: "unix"})
	// Stop when the stale socket cannot be made
	if err != nil {
		t.Fatalf("Setup: ListenUnix = %v", err)
	}
	stale.SetUnlinkOnClose(false)
	_ = stale.Close()

	l, err := u.Listen(relay.DNS)

	// Require the new listener to take the stale socket's place
	if err != nil {
		t.Fatalf("Listen(%s) over a stale socket = %v, want it listening", relay.DNS, err)
	}
	_ = l.Close()
}

// A socket another listener still serves is left alone and the new listener turned away.
func TestUnixListenRefusesALiveSocket(t *testing.T) {
	u := relay.Unix{Dir: socketDir(t)}
	first, err := u.Listen(relay.SSH)
	// Stop when the first listener cannot listen
	if err != nil {
		t.Fatalf("Setup: Listen(%s) = %v", relay.SSH, err)
	}
	t.Cleanup(func() { _ = first.Close() })

	_, err = u.Listen(relay.SSH)

	// Require the second listener to be refused
	if !errors.Is(err, relay.ErrSocketInUse) {
		t.Errorf("Listen(%s) twice = %v, want ErrSocketInUse", relay.SSH, err)
	}
}

// A folder whose socket path outgrows the kernel's limit is turned away before any socket is made.
func TestUnixListenRefusesALongPath(t *testing.T) {
	u := relay.Unix{Dir: filepath.Join(socketDir(t), strings.Repeat("d", 120))}

	_, err := u.Listen(relay.Proxy)

	// Require the refusal to name the long path
	if !errors.Is(err, relay.ErrPathTooLong) {
		t.Errorf("Listen(%s) in a deep folder = %v, want ErrPathTooLong", relay.Proxy, err)
	}
}

// A service outside the relay's set gets no socket.
func TestUnixListenRefusesAnUnknownService(t *testing.T) {
	u := relay.Unix{Dir: socketDir(t)}

	_, err := u.Listen(relay.Service(fixture.AbsentName))

	// Require the refusal to name the unknown service
	if !errors.Is(err, relay.ErrUnknownService) {
		t.Errorf("Listen(%s) = %v, want ErrUnknownService", fixture.AbsentName, err)
	}
}

// A socket lets its owner and group connect and no one else.
func TestUnixListenOpensTheSocketToOwnerAndGroup(t *testing.T) {
	u := relay.Unix{Dir: socketDir(t)}
	l, err := u.Listen(relay.Proxy)
	// Stop when the service cannot listen
	if err != nil {
		t.Fatalf("Setup: Listen(%s) = %v", relay.Proxy, err)
	}
	t.Cleanup(func() { _ = l.Close() })

	info, err := os.Stat(u.Path(relay.Proxy))

	// Require the socket's mode to admit owner and group alone
	if err != nil || info.Mode().Perm() != 0o660 {
		t.Errorf("socket mode = %v, %v, want %v", info.Mode().Perm(), err, os.FileMode(0o660))
	}
}

// When the cage's end closes its writing side, the target reads to its end and its reply still comes back.
func TestRelayPassesAHalfClose(t *testing.T) {
	u := relay.Unix{Dir: socketDir(t)}
	var lc net.ListenConfig
	target, err := lc.Listen(t.Context(), "unix", filepath.Join(socketDir(t), "target.sock"))
	// Stop when the target cannot listen
	if err != nil {
		t.Fatalf("Setup: target Listen = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })
	go func() {
		c, err := target.Accept()
		// Leave the target idle when nothing arrives
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.ReadAll(c)
		_, _ = c.Write([]byte(fixture.Reply))
	}()
	r := relay.New(u)
	_ = r.Forward(relay.SSH, func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", target.Addr().String())
	})
	serving(t, r)
	c := dialing(t, u, relay.SSH)

	_, _ = c.Write([]byte(fixture.Message))
	_ = c.(*net.UnixConn).CloseWrite()
	got := readAll(t, c)

	// Require the reply sent after the half-close to arrive
	if got != fixture.Reply {
		t.Errorf("read after half-close %q, want %q", got, fixture.Reply)
	}
}

// When the cage's side removes the socket, the relay listens again and new connections get through.
func TestServeListensAgainAfterTheSocketGoes(t *testing.T) {
	u := relay.Unix{Dir: socketDir(t)}
	r := relay.New(u)
	_ = r.Handle(relay.Proxy, answering(fixture.Reply))
	serving(t, r)
	_ = dialing(t, u, relay.Proxy).Close()

	_ = os.Remove(u.Path(relay.Proxy))
	time.Sleep(50 * time.Millisecond)
	got := readAll(t, dialing(t, u, relay.Proxy))

	// Require a connection after the removal to be answered
	if got != fixture.Reply {
		t.Errorf("read after the socket went %q, want %q", got, fixture.Reply)
	}
}
