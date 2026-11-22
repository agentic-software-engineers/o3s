package vm

import (
	"errors"
	"strings"
	"testing"
)

// The split sends log records to the log, keeps the last failure's message, and passes every other line on, a line
// that redraws itself included, written in any pieces.
func TestRecordsSplitTheLogFromWhatTheUserReads(t *testing.T) {
	var user, log strings.Builder
	r := &records{user: &user, log: &log}
	out := `{"level":"info","msg":"Starting the instance"}
{"level":"warning","msg":"invalid timezone"}
1.00 MiB / 819.29 MiB [--] 0.12%` + "\r" + `819.29 MiB / 819.29 MiB [--] 100.00%
{"level":"fatal","msg":"failed to download the image"}
inside the VM`

	// Write the output in pieces, as a pipe hands it over
	for i := 0; i < len(out); i += 7 {
		_, _ = r.Write([]byte(out[i:min(i+7, len(out))]))
	}
	r.flush()

	// Require the progress and the command's own line for the user, with the records kept out
	if want := "1.00 MiB / 819.29 MiB [--] 0.12%\r819.29 MiB / 819.29 MiB [--] 100.00%\ninside the VM"; user.String() != want {
		t.Errorf("user = %q, want %q", user.String(), want)
	}
	// Require every record in the log
	if strings.Count(log.String(), `"level"`) != 3 {
		t.Errorf("log = %q, want the three records", log.String())
	}
	// Require the failure's own words
	if r.failure != "failed to download the image" {
		t.Errorf("failure = %q, want the fatal record's message", r.failure)
	}
}

// A failure the manager names reads in its words and still unwraps to the exit it ended in.
func TestErrorReadsInTheManagersWords(t *testing.T) {
	exit := errors.New("exit status 1")
	err := error(&Error{Msg: "failed to download the image", Err: exit})

	// Require the manager's words and the exit beneath them
	if err.Error() != "failed to download the image" || !errors.Is(err, exit) {
		t.Errorf("Error = %q, unwraps to the exit %v", err, errors.Is(err, exit))
	}
}

// The qemu engine needs QEMU's tools on PATH, named with how to install them, and the other engines are ready
// as they are.
func TestCheckNamesTheMissingQEMU(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := Lima{engine: "qemu"}.Check()

	// Require QEMU named with how to install it
	if !errors.Is(err, ErrNoQEMU) || !strings.Contains(err.Error(), "install it as in:") {
		t.Errorf("Check() = %v, want %v with how to install it", err, ErrNoQEMU)
	}
	// Let an engine that ships with its system pass
	if err := (Lima{engine: "vz"}).Check(); err != nil {
		t.Errorf("Check() for vz = %v, want nil", err)
	}
}
