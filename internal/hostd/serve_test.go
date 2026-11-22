//go:build unix

package hostd_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/hostd"
)

// running serves the home at the place until the test ends, and hands back the channel Serve's result arrives on.
func running(t *testing.T, p hostd.Place) <-chan error {
	t.Helper()
	t.Setenv("O3S_SECRETS_DIR", shortDir(t))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- hostd.Serve(ctx, p, hostd.ServeOptions{Version: fixture.Version, Log: slog.New(slog.DiscardHandler)})
	}()
	t.Cleanup(cancel)
	return done
}

// answering waits for the control socket at the place to answer.
func answering(t *testing.T, p hostd.Place) hostd.Info {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	// Ask again until the process answers or the time runs out
	for {
		info, err := hostd.GetInfo(t.Context(), p.Control())
		// Hand back the first answer
		if err == nil {
			return info
		}
		// Stop once the process has had time enough to answer
		if time.Now().After(deadline) {
			t.Fatalf("Setup: GetInfo = %v, want an answer", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A second process for a home that one already serves is turned away.
func TestServeHoldsTheHomeForOneProcess(t *testing.T) {
	p := aPlace(t)
	running(t, p)
	_ = answering(t, p)

	err := hostd.Serve(t.Context(), p, hostd.ServeOptions{Version: fixture.Version, Log: slog.New(slog.DiscardHandler)})

	// Require the second process refused
	if !errors.Is(err, hostd.ErrHeld) {
		t.Errorf("second Serve = %v, want ErrHeld", err)
	}
}

// A shutdown ends the process cleanly and leaves its control socket gone and its home free.
func TestServeStopsOnAShutdown(t *testing.T) {
	p := aPlace(t)
	done := running(t, p)
	info := answering(t, p)

	_ = hostd.Shutdown(t.Context(), p.Control())

	// Require a clean stop
	select {
	case err := <-done:
		// Report a stop that claims a failure
		if err != nil {
			t.Errorf("Serve after shutdown = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve keeps running after its shutdown")
	}
	_, statErr := os.Stat(p.Control())
	lock, lockErr := hostd.TryLock(p.Lock())
	// Require the socket gone and the home free
	if !errors.Is(statErr, os.ErrNotExist) || lockErr != nil || info.PID != os.Getpid() {
		t.Errorf("after shutdown: socket %v, lock %v, served pid %d, want gone, free and %d", statErr, lockErr, info.PID, os.Getpid())
	}
	// Let go of the test's own hold
	if lock != nil {
		_ = lock.Release()
	}
}
