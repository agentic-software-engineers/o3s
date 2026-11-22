package cagebridge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
)

// ErrRefused reports a tunnel the proxy turns away.
var ErrRefused = errors.New("the proxy refused the tunnel")

// closeWriter is a connection that can end its writing side alone.
type closeWriter interface {
	CloseWrite() error
}

// Connect opens a tunnel to the host and port through the proxy socket and carries the streams through it, as an ssh
// ProxyCommand does, until the far side ends.
func Connect(ctx context.Context, sock, host, port string, in io.Reader, out io.Writer) error {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", sock)
	// Hand back a proxy out of reach
	if err != nil {
		return err
	}
	defer c.Close()
	addr := net.JoinHostPort(host, port)
	// Hand back a request that fails to go out
	if _, err := fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr); err != nil {
		return err
	}
	r := bufio.NewReader(c)
	resp, err := http.ReadResponse(r, &http.Request{Method: http.MethodConnect})
	// Hand back an answer that fails to arrive
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	// Turn away a tunnel the proxy refuses
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s to %s", ErrRefused, resp.Status, addr)
	}
	go func() {
		_, _ = io.Copy(c, in)
		// Pass the end of the input on as a half-close where the socket allows it
		if cw, ok := c.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
	}()
	_, err = io.Copy(out, r)
	return err
}
