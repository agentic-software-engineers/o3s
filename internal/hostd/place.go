package hostd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hansehart/o3s/internal/paths"
)

// maxSocketPath is the longest socket path every system the host may run takes: 104 bytes on macOS and the BSDs,
// 108 on Linux, less the closing zero.
const maxSocketPath = 103

// longestSocket is the longest socket name the host process places in its folder.
const longestSocket = "control.sock"

// ErrPathTooLong reports a socket folder whose socket paths outgrow the kernel's limit.
var ErrPathTooLong = errors.New("socket path too long")

// Place is where a home's host process keeps its sockets, its lock and its logs.
type Place struct {
	// Home is the home's folder.
	Home string
	// State is the home's own state folder, which holds the lock and the logs.
	State string
	// Sockets is the folder the sockets live in: a folder of the home's own in the user's runtime folder where the
	// system has one, since that folder is local and starts empty on every boot, and the state folder otherwise.
	Sockets string
}

// PlaceOf settles where the home's host process keeps its files, making the folders open to their owner alone.
func PlaceOf(home string) (Place, error) {
	p := Place{Home: home, State: paths.Home(home), Sockets: paths.Home(home)}
	// Take a folder of the home's own in the runtime folder where the system names one
	if runtime := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(runtime) {
		p.Sockets = filepath.Join(runtime, "o3s", paths.ID(home))
	}
	// Turn away a folder whose socket paths the kernel would cut short
	if path := filepath.Join(p.Sockets, longestSocket); len(path) > maxSocketPath {
		return Place{}, fmt.Errorf("%w: %s", ErrPathTooLong, path)
	}
	// Make each folder, open to its owner alone
	for _, dir := range []string{p.State, p.Sockets} {
		// Hand back a folder that fails to appear
		if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:gosec // the user's own runtime and state folders
			return Place{}, err
		}
	}
	return p, nil
}

// Lock is the file the process holds while it runs.
func (p Place) Lock() string { return filepath.Join(p.State, "host.lock") }

// Control is the socket the CLI reaches the process on.
func (p Place) Control() string { return filepath.Join(p.Sockets, "control.sock") }

// Log is the process's log.
func (p Place) Log() string { return filepath.Join(p.State, "host.log") }

// Crash is where the process's own output lands, which holds what it says before its log opens.
func (p Place) Crash() string { return filepath.Join(p.State, "host.out") }
