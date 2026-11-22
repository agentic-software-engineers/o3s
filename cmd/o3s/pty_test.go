package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/hansehart/o3s/internal/ptytest"
)

// sessionKey is where a script keeps its session among the values testscript hands its commands.
type sessionKey struct{}

// envKey is where a script keeps the environment it started with, for the process it starts in a pseudo-terminal.
type envKey struct{}

// script is a script's hold on o3s in a pseudo-terminal, and whether the script waited for it to leave.
type script struct {
	s      *ptytest.Session
	waited bool
}

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
	sc := &script{}
	env.Values[sessionKey{}] = sc
	env.Defer(func() {
		// Stop o3s where the script left it running
		if sc.s != nil && !sc.waited {
			sc.s.Stop()
		}
	})
}

// sessionOf is the script's hold on o3s, stopping a script whose session is missing where it has to run.
func sessionOf(ts *testscript.TestScript, started bool) *script {
	sc, _ := ts.Value(sessionKey{}).(*script)
	// Stop a script whose session is missing or still unstarted where it has to run
	if sc == nil || started && sc.s == nil {
		ts.Fatalf("no o3s in a pseudo-terminal, start it with pty")
	}
	return sc
}

// cmdPty starts o3s with the arguments given in a pseudo-terminal of the size given, 80x24 by default.
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
	// Turn away a start that lacks a command
	if len(args) == 0 {
		ts.Fatalf("usage: pty [-size=COLSxROWS] o3s [args...]")
	}
	sc := sessionOf(ts, false)
	s, err := ptytest.Start(ts.MkAbs("."), environ(ts), cols, rows, lookPath(ts, args[0]), args[1:]...)
	ts.Check(err)
	sc.s = s
}

// size reads a size the way a script writes it, as COLSxROWS.
func size(ts *testscript.TestScript, text string) (int, int) {
	cols, rows, err := ptytest.Size(text)
	ts.Check(err)
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

// environ is the script's environment as it stands now.
func environ(ts *testscript.TestScript) []string {
	vars, _ := ts.Value(envKey{}).([]string)
	out := make([]string, 0, len(vars))
	// Take every variable with the value the script gives it now
	for _, v := range vars {
		name, _, _ := strings.Cut(v, "=")
		out = append(out, name+"="+ts.Getenv(name))
	}
	return out
}

// cmdKeys presses keys in o3s's terminal, one after another: a key by its name, any other word typed as it reads.
func cmdKeys(ts *testscript.TestScript, neg bool, args []string) {
	// Turn away negated keys
	if neg {
		ts.Fatalf("unsupported: ! keys")
	}
	ts.Check(sessionOf(ts, true).s.Press(args...))
}

// cmdScreen waits until o3s's screen matches the pattern, and with ! requires it to stay free of it.
func cmdScreen(ts *testscript.TestScript, neg bool, args []string) {
	// Turn away a call that lacks a pattern
	if len(args) != 1 {
		ts.Fatalf("usage: [!] screen PATTERN")
	}
	re, err := regexp.Compile("(?m)" + args[0])
	ts.Check(err)
	s := sessionOf(ts, true).s
	// Check once the screen settled where the pattern has to stay away
	if neg {
		time.Sleep(300 * time.Millisecond)
		// Stop at a screen that shows the pattern
		if re.MatchString(s.Screen()) {
			ts.Fatalf("screen shows %q:\n%s", args[0], s.Screen())
		}
		return
	}
	// Stop once the screen shows the pattern, or once the wait is over
	if !s.Until(re.MatchString) {
		ts.Fatalf("screen lacks %q after %v:\n%s", args[0], ptytest.Wait(), s.Screen())
	}
}

// cmdPtySize resizes o3s's terminal, the way a user's window does.
func cmdPtySize(ts *testscript.TestScript, neg bool, args []string) {
	// Turn away a negated or bare resize
	if neg || len(args) != 1 {
		ts.Fatalf("usage: ptysize COLSxROWS")
	}
	cols, rows := size(ts, args[0])
	ts.Check(sessionOf(ts, true).s.Resize(cols, rows))
}

// cmdPtyWait waits for o3s to leave, and requires it to succeed, or with ! to fail.
func cmdPtyWait(ts *testscript.TestScript, neg bool, _ []string) {
	sc := sessionOf(ts, true)
	left, err := sc.s.Left()
	// Stop where o3s keeps running past the wait
	if !left {
		ts.Fatalf("o3s still runs after %v, on:\n%s", ptytest.Wait(), sc.s.Screen())
	}
	sc.waited = true
	// Check how o3s left against what the script expects
	switch {
	case neg && err == nil:
		ts.Fatalf("o3s succeeded, want a failure")
	case !neg && ptytest.Failed(err):
		ts.Fatalf("o3s left with %v, on:\n%s", err, sc.s.Screen())
	case !neg && err != nil:
		ts.Fatalf("o3s: %v", err)
	}
}
