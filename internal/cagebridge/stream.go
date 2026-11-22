// Package cagebridge runs inside the cage and carries its loopback to the host's sockets: the proxy port and the DNS
// port the cage's tools talk to, and the tunnel ssh opens through the proxy.
package cagebridge

import (
	"context"
	"net"
	"sync"

	"github.com/hansehart/o3s/internal/relay"
)

// Stream carries every connection the listener takes to the socket and back, until the context ends.
func Stream(ctx context.Context, l net.Listener, sock string) error {
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	// Take connections until the listener closes
	for {
		c, err := l.Accept()
		// Report a clean stop once the context ends, and any other failure as it is
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		wg.Go(func() { carry(ctx, c, sock) })
	}
}

// carry pipes one connection to the socket until both directions end, or the context ends.
func carry(ctx context.Context, c net.Conn, sock string) {
	defer c.Close()
	var d net.Dialer
	up, err := d.DialContext(ctx, "unix", sock)
	// Drop a connection whose socket stays out of reach
	if err != nil {
		return
	}
	defer up.Close()
	stop := context.AfterFunc(ctx, func() { _ = c.Close(); _ = up.Close() })
	defer stop()
	relay.Pipe(c, up)
}
