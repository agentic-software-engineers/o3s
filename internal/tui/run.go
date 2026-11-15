package tui

import (
	"bufio"
	"io"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// lineMsg is one line a running command printed.
type lineMsg string

// endMsg is the end of a running command, and how it ended.
type endMsg struct {
	err error
}

// stream carries a captured command's lines and then its end to the app, one message at a time.
type stream chan tea.Msg

// next waits for the stream's next message.
func (s stream) next() tea.Cmd {
	return func() tea.Msg { return <-s }
}

// capture runs a command with stdout and stderr piped into a stream line by line, the value given on stdin, and its end last.
func capture(cmd *exec.Cmd, stdin string) stream {
	s := make(stream, 64)
	r, w := io.Pipe()
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = w, w
	go func() {
		// Report a command that fails to start
		if err := cmd.Start(); err != nil {
			s <- endMsg{err: err}
			return
		}
		done := make(chan error, 1)
		go func() {
			err := cmd.Wait()
			_ = w.Close()
			done <- err
		}()
		lines := bufio.NewScanner(r)
		// Pass every line on as it comes, until the command closes its output
		for lines.Scan() {
			s <- lineMsg(lines.Text())
		}
		// Name output that fails to read
		if err := lines.Err(); err != nil {
			s <- lineMsg("✗ " + err.Error())
		}
		s <- endMsg{err: <-done}
	}()
	return s
}

// inTerminal wraps a command that needs the real terminal, so its output stays on screen until the user goes back.
func inTerminal(cmd *exec.Cmd) *exec.Cmd {
	script := `"$@"; status=$?; printf '\n↵ back to the menu '; read -r _; exit $status`
	wrapped := exec.Command("sh", append([]string{"-c", script, "sh", cmd.Path}, cmd.Args[1:]...)...) //nolint:gosec,noctx // the menu runs its own binary, which the app's context ends with the program
	wrapped.Env, wrapped.Dir = cmd.Env, cmd.Dir
	return wrapped
}
