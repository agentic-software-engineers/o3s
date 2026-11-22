package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

// hostPID reads the pid o3s host status prints.
var hostPID = regexp.MustCompile(`pid (\d+)`)

// hostCmds are the script commands that end and wait for the home's host process.
var hostCmds = map[string]func(ts *testscript.TestScript, neg bool, args []string){
	"killhost":  cmdKillHost,
	"hostgone":  cmdHostGone,
	"hoststays": cmdHostStays,
}

// cmdKillHost ends the home's host process at once, as a crash would, and waits for it to fall silent.
func cmdKillHost(ts *testscript.TestScript, _ bool, _ []string) {
	ts.Check(ts.Exec("o3s", "host", "status"))
	m := hostPID.FindStringSubmatch(ts.ReadFile("stdout"))
	// Stop when the status lacks a pid
	if m == nil {
		ts.Fatalf("o3s host status names no pid")
	}
	pid, _ := strconv.Atoi(m[1])
	proc, err := os.FindProcess(pid)
	ts.Check(err)
	ts.Check(proc.Kill())
	cmdHostGone(ts, false, nil)
}

// cmdHostGone waits for the home's host process to fall silent.
func cmdHostGone(ts *testscript.TestScript, _ bool, _ []string) {
	deadline := time.Now().Add(5 * time.Second)
	// Ask again until the process falls silent or the time runs out
	for ts.Exec("o3s", "host", "status") == nil {
		// Stop once the process has had time enough to go
		if time.Now().After(deadline) {
			ts.Fatalf("the host process keeps answering")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cmdHostStays requires the home's host process to keep answering for a while.
func cmdHostStays(ts *testscript.TestScript, _ bool, _ []string) {
	// Ask several times over the while
	for range 10 {
		// Stop once the process falls silent
		if err := ts.Exec("o3s", "host", "status"); err != nil {
			ts.Fatalf("the host process fell silent: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// hostSetup stops any host process a script leaves running once the script ends.
func hostSetup(env *testscript.Env) {
	env.Defer(func() {
		cmd := exec.Command(filepath.Join(env.Getenv("O3S_DIR"), "o3s"), "host", "stop") //nolint:noctx,gosec // the script has ended with its context, and runs the o3s testscript built
		cmd.Dir, cmd.Env = env.WorkDir, env.Vars
		_ = cmd.Run()
	})
}
