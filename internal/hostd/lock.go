// Package hostd keeps one o3s host process running for each home: it holds the home for one process, places its
// sockets and log, serves its control socket, and starts it again from the CLI whenever it has gone.
package hostd

import (
	"errors"
	"os"
)

// ErrHeld reports a home another host process holds already.
var ErrHeld = errors.New("another host process holds this home")

// Lock is the hold one host process keeps on its home for as long as it runs.
type Lock struct {
	f *os.File
}

// TryLock takes the hold on the lock file at once, and reports ErrHeld while another process keeps it.
func TryLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	// Hand back a lock file that fails to open
	if err != nil {
		return nil, err
	}
	// Close the file again when the hold is refused
	if err := tryLock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Release lets go of the hold.
func (l *Lock) Release() error {
	_ = unlock(l.f)
	return l.f.Close()
}
