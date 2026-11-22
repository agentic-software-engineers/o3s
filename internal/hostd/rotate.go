package hostd

import (
	"os"
	"sync"
)

// RotatingFile is a log that moves aside to one older file once it reaches its limit, and starts afresh.
type RotatingFile struct {
	path  string
	limit int64

	mu   sync.Mutex
	f    *os.File
	size int64
}

// Rotating opens the log at the path for appending, to move aside at the limit in bytes.
func Rotating(path string, limit int64) (*RotatingFile, error) {
	r := &RotatingFile{path: path, limit: limit}
	// Hand back a log that fails to open
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

// open opens the log for appending and notes how much it holds.
func (r *RotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	// Hand back a log that fails to open
	if err != nil {
		return err
	}
	info, err := f.Stat()
	// Hand back a log whose size stays unknown
	if err != nil {
		_ = f.Close()
		return err
	}
	r.f, r.size = f, info.Size()
	return nil
}

// Write appends the bytes, moving the log aside first when they would take it past its limit.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Move the full log aside before it passes its limit
	if r.size > 0 && r.size+int64(len(p)) > r.limit {
		// Hand back a log that fails to move aside
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate closes the log, puts it in place of the older file, and opens a fresh one.
func (r *RotatingFile) rotate() error {
	_ = r.f.Close()
	// Hand back a log that fails to move aside
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		return err
	}
	return r.open()
}

// Close closes the log.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
