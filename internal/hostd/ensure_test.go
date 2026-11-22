package hostd_test

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/hostd"
)

// spawner starts a process for the place in-process, at the version given, and counts how often it was asked.
type spawner struct {
	t       *testing.T
	p       hostd.Place
	version string
	calls   atomic.Int32
	start   bool
}

// Spawn starts the stand-in process, where the test lets it start.
func (s *spawner) Spawn() error {
	s.calls.Add(1)
	// Start the stand-in where the test lets it
	if s.start {
		controlled(s.t, s.p, hostd.Info{Version: s.version, Home: fixture.HomeDir})
	}
	return nil
}

// ensuring is the options a test ensures a process with.
func ensuring(s *spawner) hostd.EnsureOptions {
	return hostd.EnsureOptions{Version: fixture.Version, Spawn: s.Spawn, Ready: time.Second, Gone: time.Second, Attempts: 3}
}

// A home whose process is missing gets one started, and the started process answers.
func TestEnsureStartsAMissingProcess(t *testing.T) {
	p := aPlace(t)
	s := &spawner{t: t, p: p, version: fixture.Version, start: true}

	info, err := hostd.Ensure(t.Context(), p, ensuring(s))

	// Require one start and the process's answer
	if err != nil || s.calls.Load() != 1 || info.Version != fixture.Version {
		t.Errorf("Ensure = %+v, %v after %d starts, want version %s after 1", info, err, s.calls.Load(), fixture.Version)
	}
}

// A home whose process runs at this version keeps it as the one process.
func TestEnsureKeepsARunningProcess(t *testing.T) {
	p := aPlace(t)
	controlled(t, p, hostd.Info{Version: fixture.Version})
	s := &spawner{t: t, p: p, version: fixture.Version, start: true}

	_, err := hostd.Ensure(t.Context(), p, ensuring(s))

	// Require the running process kept
	if err != nil || s.calls.Load() != 0 {
		t.Errorf("Ensure = %v after %d starts, want nil after 0", err, s.calls.Load())
	}
}

// A process of another version is asked to stop, and one of this version takes its place.
func TestEnsureReplacesAnotherVersion(t *testing.T) {
	p := aPlace(t)
	old := controlled(t, p, hostd.Info{Version: fixture.NewerVersion})
	s := &spawner{t: t, p: p, version: fixture.Version, start: true}

	info, err := hostd.Ensure(t.Context(), p, ensuring(s))

	// Require the old process gone and this version answering
	select {
	case <-old:
	default:
		t.Error("the process of the other version keeps running")
	}
	if err != nil || info.Version != fixture.Version || s.calls.Load() != 1 {
		t.Errorf("Ensure = %+v, %v after %d starts, want version %s after 1", info, err, s.calls.Load(), fixture.Version)
	}
}

// A process that stays silent is started a bounded number of times, and the failure names its output.
func TestEnsureGivesUpAfterItsAttempts(t *testing.T) {
	p := aPlace(t)
	s := &spawner{t: t, p: p}
	o := ensuring(s)
	o.Ready = 100 * time.Millisecond

	_, err := hostd.Ensure(t.Context(), p, o)

	// Require three starts and a failure that points at the output
	if !errors.Is(err, hostd.ErrUnready) || !strings.Contains(err.Error(), p.Crash()) || s.calls.Load() != 3 {
		t.Errorf("Ensure = %v after %d starts, want ErrUnready naming %s after 3", err, s.calls.Load(), p.Crash())
	}
}
