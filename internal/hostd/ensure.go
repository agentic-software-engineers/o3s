package hostd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// pollEvery is how often Ensure asks the control socket while it waits.
const pollEvery = 50 * time.Millisecond

// ErrUnready reports a host process that stayed silent through every start.
var ErrUnready = errors.New("the host process stays silent")

// EnsureOptions are what Ensure keeps a process running with.
type EnsureOptions struct {
	// Version is the o3s version the process has to run.
	Version string
	// Spawn starts a new process for the home.
	Spawn func() error
	// Ready is how long a new process may take to answer.
	Ready time.Duration
	// Gone is how long a process asked to stop may take to go before it is ended.
	Gone time.Duration
	// Attempts is how often Ensure starts a process before it gives up.
	Attempts int
}

// Ensure hands back the info of the home's process at the version wanted: the running one where it answers at that
// version, and otherwise a new one, after the running one of another version has stopped.
func Ensure(ctx context.Context, p Place, o EnsureOptions) (Info, error) {
	ctl := p.Control()
	// Start a process until one answers at the version wanted, or the attempts run out
	for attempt := range o.Attempts {
		info, err := GetInfo(ctx, ctl)
		// Keep a process that answers at the version wanted
		if err == nil && info.Version == o.Version {
			return info, nil
		}
		// Stop a process of another version before a new one takes its place
		if err == nil {
			retire(ctx, ctl, info, o.Gone)
		}
		// Hand back a start that fails
		if err := o.Spawn(); err != nil {
			return Info{}, err
		}
		info, err = await(ctx, ctl, o.Ready)
		// Hand back the new process once it answers at the version wanted
		if err == nil && info.Version == o.Version {
			return info, nil
		}
		// Stop once the context ends
		if !pause(ctx, time.Duration(attempt+1)*pollEvery) {
			return Info{}, ctx.Err()
		}
	}
	return Info{}, fmt.Errorf("%w after %d starts, see %s", ErrUnready, o.Attempts, p.Crash())
}

// retire asks the process to stop and waits for it to go, and ends it where it stays past the wait.
func retire(ctx context.Context, ctl string, info Info, gone time.Duration) {
	_ = Shutdown(ctx, ctl)
	// Leave the process be once its control socket falls silent within the wait
	if silent(ctx, ctl, gone) {
		return
	}
	// End a process that stays past the wait, keeping the one asking alive
	if info.PID > 0 && info.PID != os.Getpid() {
		// End the process where the system still knows it
		if proc, err := os.FindProcess(info.PID); err == nil {
			_ = proc.Kill()
		}
	}
}

// await asks the control socket until it answers or the wait runs out.
func await(ctx context.Context, ctl string, wait time.Duration) (Info, error) {
	deadline := time.Now().Add(wait)
	// Ask again until an answer comes or the wait runs out
	for {
		info, err := GetInfo(ctx, ctl)
		// Hand back the first answer, or the last failure once the wait runs out
		if err == nil || time.Now().After(deadline) {
			return info, err
		}
		// Stop once the context ends
		if !pause(ctx, pollEvery) {
			return Info{}, ctx.Err()
		}
	}
}

// silent asks the control socket until it stays silent, and reports whether it fell silent before the wait ran out.
func silent(ctx context.Context, ctl string, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	// Ask again while the socket still answers and the wait lasts
	for {
		// Report the socket silent once a question goes unanswered
		if _, err := GetInfo(ctx, ctl); err != nil {
			return true
		}
		// Report the socket still answering once the wait runs out, or the context ends
		if time.Now().After(deadline) || !pause(ctx, pollEvery) {
			return false
		}
	}
}

// pause waits for the given while and reports false when the context ends first.
func pause(ctx context.Context, wait time.Duration) bool {
	t := time.NewTimer(wait)
	defer t.Stop()
	// Wait for the timer, or stop once the context ends
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
