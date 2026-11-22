//go:build unix

package hostd

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock takes an exclusive hold on the file at once.
func tryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	// Name a hold another process keeps
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrHeld
	}
	return err
}

// unlock lets go of the hold on the file.
func unlock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
