package cli

import (
	"os"
	"testing"
)

// The null device and a pipe read as streams with nobody behind them, so a confirmation asks to be given up front.
func TestIsTerminalTellsATerminalFromTheNullDeviceAndAPipe(t *testing.T) {
	null, err := os.Open(os.DevNull)
	// Stop when the null device fails to open
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = null.Close() })
	r, w, err := os.Pipe()
	// Stop when the pipe fails to open
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	// Check every stream that reaches no terminal
	for name, stream := range map[string]*os.File{"the null device": null, "a pipe": r} {
		// Report a stream read as a terminal
		if isTerminal(stream) {
			t.Errorf("isTerminal(%s) = true, want false", name)
		}
	}
}
