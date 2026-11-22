package hostd_test

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/hostd"
)

// aPlace settles a home's place in short folders of the test's own.
func aPlace(t *testing.T) hostd.Place {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", shortDir(t))
	t.Setenv("XDG_STATE_HOME", shortDir(t))
	p, err := hostd.PlaceOf(fixture.HomeDir)
	// Stop when the place fails to settle
	if err != nil {
		t.Fatalf("Setup: PlaceOf = %v", err)
	}
	return p
}

// controlled serves the control socket at the place with the info until the test ends or a shutdown arrives, and
// hands back a channel that closes once it stops.
func controlled(t *testing.T, p hostd.Place, info hostd.Info) <-chan struct{} {
	t.Helper()
	return serveControl(t, p, info, true)
}

// serveControl serves the control socket at the place with the info until the test ends, and until a shutdown arrives
// where it obeys one, and hands back a channel that closes once it stops.
func serveControl(t *testing.T, p hostd.Place, info hostd.Info, obey bool) <-chan struct{} {
	t.Helper()
	l, err := hostd.ListenControl(t.Context(), p.Control())
	// Stop when the control socket fails to listen
	if err != nil {
		t.Fatalf("Setup: ListenControl = %v", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	onShutdown := stop
	// Take a shutdown and stay where the server ignores it
	if !obey {
		onShutdown = func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = hostd.ServeControl(ctx, l, info, onShutdown, slog.New(slog.DiscardHandler))
	}()
	t.Cleanup(func() {
		stop()
		<-done
	})
	return done
}

// The control socket answers with the process's version, pid and home.
func TestControlAnswersWithTheProcessInfo(t *testing.T) {
	p := aPlace(t)
	want := hostd.Info{Version: fixture.Version, PID: os.Getpid(), Home: fixture.HomeDir}
	controlled(t, p, want)

	got, err := hostd.GetInfo(t.Context(), p.Control())

	// Require the very info the process serves
	if diff := cmp.Diff(want, got); diff != "" || err != nil {
		t.Errorf("GetInfo = %v (-want +got):\n%s", err, diff)
	}
}

// A shutdown through the control socket ends the process.
func TestControlShutdownEndsTheProcess(t *testing.T) {
	p := aPlace(t)
	done := controlled(t, p, hostd.Info{Version: fixture.Version})

	err := hostd.Shutdown(t.Context(), p.Control())

	// Require the shutdown answered and the server gone soon after
	if err != nil {
		t.Fatalf("Shutdown = %v, want nil", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the control socket keeps serving after its shutdown")
	}
}

// A socket a crashed process left behind gives way to the new listener.
func TestListenControlReplacesAStaleSocket(t *testing.T) {
	p := aPlace(t)
	var lc net.ListenConfig
	stale, err := lc.Listen(t.Context(), "unix", p.Control())
	// Stop when the stale socket fails to appear
	if err != nil {
		t.Fatalf("Setup: Listen = %v", err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = stale.Close()

	l, err := hostd.ListenControl(t.Context(), p.Control())

	// Require the new listener in the stale socket's place
	if err != nil {
		t.Fatalf("ListenControl over a stale socket = %v, want it listening", err)
	}
	_ = l.Close()
}

// Asking a home whose process is gone reports the failure.
func TestGetInfoReportsAMissingProcess(t *testing.T) {
	p := aPlace(t)

	_, err := hostd.GetInfo(t.Context(), p.Control())

	// Require the failure
	if err == nil {
		t.Error("GetInfo with no process = nil, want an error")
	}
}

// The control socket lets its owner alone connect, and carries the sticky bit that keeps it through clean-ups.
func TestListenControlOpensTheSocketToItsOwnerAlone(t *testing.T) {
	p := aPlace(t)
	l, err := hostd.ListenControl(t.Context(), p.Control())
	// Stop when the control socket fails to listen
	if err != nil {
		t.Fatalf("Setup: ListenControl = %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	info, err := os.Stat(p.Control())

	// Require mode 0600 with the sticky bit
	if err != nil || info.Mode().Perm() != 0o600 || info.Mode()&os.ModeSticky == 0 {
		t.Errorf("control socket mode = %v, %v, want 0600 with the sticky bit", info.Mode(), err)
	}
}
