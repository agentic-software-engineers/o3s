//go:build unix

package hostd

import "github.com/hansehart/o3s/internal/relay"

// transport carries the cage's connections over Unix sockets in the folder.
func transport(dir string) (relay.Transport, error) {
	return relay.Unix{Dir: dir}, nil
}
