package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"github.com/rogpeppe/go-internal/testscript"
)

// The keys a script names, as a terminal sends them.
var ptyKeys = map[string]string{
	"enter":  "\r",
	"esc":    "\x1b",
	"up":     "\x1b[A",
	"down":   "\x1b[B",
	"right":  "\x1b[C",
	"left":   "\x1b[D",
	"tab":    "\t",
	"space":  " ",
	"ctrl+c": "\x03",
	"ctrl+u": "\x15",
}

// ptyWait bounds how long a script waits for a screen or for o3s to leave.
const ptyWait = 5 * time.Second

// session is o3s running in a pseudo-terminal, read back through a terminal emulator as a user's terminal shows it.
type session struct {
	cmd    *exec.Cmd
	pty    *os.File
	term   *vt.Emulator
	done   chan struct{}
	err    error
	waited bool
	mu     sync.Mutex
}

// Write hands what o3s prints to the terminal emulator, one write at a time, so the screen is read whole.
func (s *session) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.term.Write(p)
}

// screen is the text the terminal emulator shows now.
func (s *session) screen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.term.String()
}

// sessionKey is where a script keeps its session among the values testscript hands its commands.
type sessionKey struct{}

// envKey is where a script keeps the environment it started with, for the process it starts in a pseudo-terminal.
type envKey struct{}

// ptyCmds are the commands a script drives o3s in a pseudo-terminal with, as a user's terminal does: start it, press
// keys, wait for what the screen shows, resize, and wait for it to leave.
var ptyCmds = map[string]func(ts *testscript.TestScript, neg bool, args []string){
	"pty":     cmdPty,
	"keys":    cmdKeys,
	"screen":  cmdScreen,
	"ptysize": cmdPtySize,
	"ptywait": cmdPtyWait,
}

// ptySetup keeps a script's environment and a place for its session, and stops o3s once the script ends.
func ptySetup(env *testscript.Env) {
	env.Values[envKey{}] = env.Vars
	s := &session{}
	env.Values[sessionKey{}] = s
	env.Defer(func() {
		// Stop o3s where the script left it running
		if s.cmd != nil && !s.waited {
			_ = s.cmd.Process.Kill()
			<-s.done
		}
	})
}

// sessionOf is the script's session, stopping a script that drives none.
func sessionOf(ts *testscript.TestScript, started bool) *session {
	s, _ := ts.Value(sessionKey{}).(*session)
	// Stop a script whose session is missing or not started where it has to be
	if s == nil || started && s.cmd == nil {
		ts.Fatalf("no o3s in a pseudo-terminal, start it with pty")
	}
	return s
}

// cmdPty starts o3s with the arguments given in a pseudo-terminal of the size given, 80x24 where none is.
func cmdPty(ts *testscript.TestScript, neg bool, args []string) {
	// Turn away a negated start
	if neg {
		ts.Fatalf("unsupported: ! pty")
	}
	cols, rows := 80, 24
	// Read the size a script names
	if len(args) > 0 && strings.HasPrefix(args[0], "-size=") {
		cols, rows = size(ts, strings.TrimPrefix(args[0], "-size="))
		args = args[1:]
	}
	// Turn away a start without a command
	if len(args) == 0 {
		ts.Fatalf("usage: pty [-size=COLSxROWS] o3s [args...]")
	}
	s := sessionOf(ts, false)
	cmd := exec.CommandContext(context.Background(), lookPath(ts, args[0]), args[1:]...) //nolint:gosec // the scripts start the o3s under test
	cmd.Dir = ts.MkAbs(".")
	cmd.Env = environ(ts)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	ts.Check(err)
	s.cmd, s.pty, s.term, s.done = cmd, f, vt.NewEmulator(cols, rows), make(chan struct{})
	go func() { _, _ = io.Copy(s, f) }()
	go func() { _, _ = io.Copy(f, s.term) }()
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		close(s.done)
	}()
}

// size reads a size the way a script writes it, as COLSxROWS.
func size(ts *testscript.TestScript, text string) (int, int) {
	c, r, ok := strings.Cut(text, "x")
	cols, cerr := strconv.Atoi(c)
	rows, rerr := strconv.Atoi(r)
	// Stop at a size that reads as none
	if !ok || cerr != nil || rerr != nil || cols < 1 || rows < 1 {
		ts.Fatalf("size %q, want COLSxROWS", text)
	}
	return cols, rows
}

// lookPath finds a command on the script's PATH, where testscript put o3s and the fakes.
func lookPath(ts *testscript.TestScript, name string) string {
	// Look through every folder of the script's PATH
	for _, dir := range filepath.SplitList(ts.Getenv("PATH")) {
		path := filepath.Join(dir, name)
		// Take the first folder that holds the command
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	ts.Fatalf("%s is on no folder of PATH", name)
	return ""
}

// environ is the script's environment as it stands now, with a terminal that takes colors.
func environ(ts *testscript.TestScript) []string {
	vars, _ := ts.Value(envKey{}).([]string)
	out := make([]string, 0, len(vars)+1)
	// Take every variable with the value the script gives it now
	for _, v := range vars {
		name, _, _ := strings.Cut(v, "=")
		out = append(out, name+"="+ts.Getenv(name))
	}
	return append(out, "TERM=xterm-256color")
}

// cmdKeys presses keys in o3s's terminal, one after another: a key by its name, any other word typed as it reads.
func cmdKeys(ts *testscript.TestScript, neg bool, args []string) {
	// Turn away negated keys
	if neg {
		ts.Fatalf("unsupported: ! keys")
	}
	s := sessionOf(ts, true)
	// Press every key in turn, giving o3s a moment between them as a user's hands do
	for _, a := range args {
		seq, named := ptyKeys[a]
		// Type a word that names no key
		if !named {
			seq = a
		}
		_, err := io.WriteString(s.pty, seq)
		ts.Check(err)
		pause := 50 * time.Millisecond
		// Give a lone Esc the time a terminal leaves before it reads it as a key of its own, not the start of a sequence
		if seq == ptyKeys["esc"] {
			pause = 300 * time.Millisecond
		}
		time.Sleep(pause)
	}
}

// cmdScreen waits until o3s's screen matches the pattern, and with ! requires it to stay without it.
func cmdScreen(ts *testscript.TestScript, neg bool, args []string) {
	// Turn away a call without one pattern
	if len(args) != 1 {
		ts.Fatalf("usage: [!] screen PATTERN")
	}
	re, err := regexp.Compile("(?m)" + args[0])
	ts.Check(err)
	s := sessionOf(ts, true)
	// Check once the screen settled where the pattern has to stay away
	if neg {
		time.Sleep(300 * time.Millisecond)
		// Stop at a screen that shows the pattern
		if re.MatchString(s.screen()) {
			ts.Fatalf("screen shows %q:\n%s", args[0], s.screen())
		}
		return
	}
	// Stop once the screen shows the pattern, or once the wait is over
	if !waitFor(func() bool { return re.MatchString(s.screen()) }) {
		ts.Fatalf("screen lacks %q after %v:\n%s", args[0], ptyWait, s.screen())
	}
}

// waitFor checks a condition again and again until it holds, and reports whether it did within the wait, longer on
// CI's slower machines.
func waitFor(cond func() bool) bool {
	wait := ptyWait
	// Wait longer on CI
	if os.Getenv("CI") != "" {
		wait *= 3
	}
	end := time.Now().Add(wait)
	// Check until the condition holds or the wait is over
	for time.Now().Before(end) {
		// Report a condition that holds
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// cmdPtySize resizes o3s's terminal, the way a user's window does.
func cmdPtySize(ts *testscript.TestScript, neg bool, args []string) {
	// Turn away a negated or bare resize
	if neg || len(args) != 1 {
		ts.Fatalf("usage: ptysize COLSxROWS")
	}
	s := sessionOf(ts, true)
	cols, rows := size(ts, args[0])
	s.mu.Lock()
	s.term.Resize(cols, rows)
	s.mu.Unlock()
	ts.Check(pty.Setsize(s.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})) //nolint:gosec // the sizes come from the script
}

// cmdPtyWait waits for o3s to leave, and requires it to succeed, or with ! to fail.
func cmdPtyWait(ts *testscript.TestScript, neg bool, _ []string) {
	s := sessionOf(ts, true)
	// Stop where o3s keeps running past the wait
	if !waitFor(func() bool {
		select {
		case <-s.done:
			return true
		default:
			return false
		}
	}) {
		ts.Fatalf("o3s still runs after %v, on:\n%s", ptyWait, s.screen())
	}
	s.waited = true
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	var exit *exec.ExitError
	// Check how o3s left against what the script expects
	switch {
	case neg && err == nil:
		ts.Fatalf("o3s succeeded, want a failure")
	case !neg && errors.As(err, &exit):
		ts.Fatalf("o3s left with %v, on:\n%s", err, s.screen())
	case !neg && err != nil:
		ts.Fatalf("o3s: %v", err)
	}
}
