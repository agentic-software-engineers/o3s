package vm

import (
	"bytes"
	"encoding/json"
	"io"
)

// records splits what the VM manager prints on stderr: its log records, which it writes as JSON, go to the log, and
// every other line, such as a download's progress or a command's own errors, goes on to the user. It keeps the message
// of the last record that names a failure.
type records struct {
	user, log io.Writer
	failure   string
	pending   []byte
}

// record is the part of a log record the split reads.
type record struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// Write takes what the VM manager printed, passing on every line it completes.
func (r *records) Write(p []byte) (int, error) {
	r.pending = append(r.pending, p...)
	// Pass on every line the output completes, a carriage return ending a line that redraws itself
	for {
		i := bytes.IndexAny(r.pending, "\r\n")
		// Wait for the rest of a line still being written
		if i < 0 {
			return len(p), nil
		}
		r.line(r.pending[:i+1])
		r.pending = r.pending[i+1:]
	}
}

// flush passes on a last line the output left open.
func (r *records) flush() {
	// Pass on what remains
	if len(r.pending) > 0 {
		r.line(r.pending)
		r.pending = nil
	}
}

// line sends one line where it belongs: a log record to the log, keeping a failure's message, and anything else to the
// user.
func (r *records) line(l []byte) {
	var rec record
	// Pass on a line that fails to read as a log record
	if json.Unmarshal(bytes.TrimSpace(l), &rec) != nil || rec.Level == "" {
		_, _ = r.user.Write(l)
		return
	}
	_, _ = r.log.Write(l)
	// Keep the message of a record that names a failure
	if rec.Level == "fatal" || rec.Level == "error" {
		r.failure = rec.Msg
	}
}

// Error is a failure the VM manager named in its own words, with the exit it ended in.
type Error struct {
	Msg string
	Err error
}

// Error is the VM manager's own words for the failure.
func (e *Error) Error() string {
	return e.Msg
}

// Unwrap hands back the exit the VM manager ended in.
func (e *Error) Unwrap() error {
	return e.Err
}
