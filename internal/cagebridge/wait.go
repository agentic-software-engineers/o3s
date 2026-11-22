package cagebridge

import (
	"context"
	"os"
	"time"
)

// waitEvery is how often WaitFor looks for the sockets.
const waitEvery = 50 * time.Millisecond

// WaitFor waits until every path is a socket, or the context ends.
func WaitFor(ctx context.Context, paths ...string) error {
	t := time.NewTicker(waitEvery)
	defer t.Stop()
	// Look at each path in turn until it is a socket
	for _, path := range paths {
		// Look again on every tick until the path is a socket
		for !isSocket(path) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
			}
		}
	}
	return nil
}

// isSocket reports whether the path is a socket.
func isSocket(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}
