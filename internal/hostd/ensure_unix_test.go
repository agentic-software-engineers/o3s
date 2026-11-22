//go:build unix

package hostd_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/hostd"
)

// stubborn serves the control socket at the place with the info until the test ends, taking every shutdown and
// staying all the same.
func stubborn(t *testing.T, p hostd.Place, info hostd.Info) {
	t.Helper()
	serveControl(t, p, info, false)
}

// sleeper starts a process that idles until the test ends, and hands back its pid and a channel that closes once it
// ends.
func sleeper(t *testing.T) (int, <-chan struct{}) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "sleep", "60")
	// Stop when the process fails to start
	if err := cmd.Start(); err != nil {
		t.Fatalf("Setup: start sleep = %v", err)
	}
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		_ = cmd.Wait()
	}()
	return cmd.Process.Pid, ended
}

// A process of another version that stops when asked is left to end itself.
func TestEnsureLetsAnObedientProcessEndItself(t *testing.T) {
	p := aPlace(t)
	pid, ended := sleeper(t)
	controlled(t, p, hostd.Info{Version: fixture.NewerVersion, PID: pid})
	s := &spawner{t: t, p: p, version: fixture.Version, start: true}

	_, err := hostd.Ensure(t.Context(), p, ensuring(s))

	// Require the process behind the old pid untouched
	select {
	case <-ended:
		t.Errorf("Ensure = %v, and it ended the process that stopped when asked", err)
	case <-time.After(300 * time.Millisecond):
	}
}

// A process of another version that keeps answering after its shutdown is ended once the wait runs out.
func TestEnsureEndsAProcessThatStaysAfterItsShutdown(t *testing.T) {
	p := aPlace(t)
	pid, ended := sleeper(t)
	stubborn(t, p, hostd.Info{Version: fixture.NewerVersion, PID: pid})
	s := &spawner{t: t, p: p, version: fixture.Version, start: true}
	o := ensuring(s)
	o.Gone = 200 * time.Millisecond

	info, err := hostd.Ensure(t.Context(), p, o)

	// Require the process behind the old pid ended, and this version answering
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the process that stayed after its shutdown keeps running")
	}
	if err != nil || info.Version != fixture.Version {
		t.Errorf("Ensure = %+v, %v, want version %s", info, err, fixture.Version)
	}
}
