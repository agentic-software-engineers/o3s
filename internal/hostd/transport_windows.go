//go:build windows

package hostd

import (
	"errors"

	"github.com/hansehart/o3s/internal/relay"
)

// errAwaitsTransport reports a system whose cage transport arrives with its VM.
var errAwaitsTransport = errors.New("the host process on Windows serves the cage once its VM transport arrives")

// transport reports the cage transport this system awaits.
func transport(string) (relay.Transport, error) {
	return nil, errAwaitsTransport
}
