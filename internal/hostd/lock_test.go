package hostd_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hansehart/o3s/internal/hostd"
)

// A second hold on a held lock is refused at once, and taken once the first lets go.
func TestTryLockHoldsTheHomeForOneProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.lock")
	first, err := hostd.TryLock(path)
	// Stop when the first hold fails
	if err != nil {
		t.Fatalf("Setup: TryLock = %v", err)
	}

	_, second := hostd.TryLock(path)
	_ = first.Release()
	third, after := hostd.TryLock(path)

	// Require the refusal while held and the hold once free
	if !errors.Is(second, hostd.ErrHeld) || after != nil {
		t.Errorf("TryLock while held = %v, after release = %v, want ErrHeld then nil", second, after)
	}
	// Let go of the last hold
	if third != nil {
		_ = third.Release()
	}
}
