package tui

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// lineMsg is one line a running command printed.
type lineMsg string

// ErrorFileEnv names where the menu asks a command on the real terminal to leave the reason it failed.
const ErrorFileEnv = "O3S_ERROR_FILE"

// endMsg is the end of a running command, and how it ended.
type endMsg struct {
	err error
	// reason is the error a command on the real terminal left behind.
	reason string
	// told marks a captured command whose output already named its error.
	told bool
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
		told := false
		// Pass every line on as it comes, until the command closes its output
		for lines.Scan() {
			told = told || strings.HasPrefix(strings.TrimSpace(lines.Text()), "error:")
			s <- lineMsg(lines.Text())
		}
		// Name output that fails to read
		if err := lines.Err(); err != nil {
			s <- lineMsg("error: " + err.Error())
		}
		s <- endMsg{err: <-done, told: told}
	}()
	return s
}

// inTerminalLeaving runs a command on the real terminal and keeps the reason it gives for failing.
func inTerminalLeaving(cmd *exec.Cmd, hand func(*exec.Cmd, tea.ExecCallback) tea.Cmd) tea.Cmd {
	file, err := os.CreateTemp("", "o3s-error-*")
	// Run the command as before where the handoff is out of reach
	if err != nil {
		return hand(inTerminal(cmd), func(err error) tea.Msg { return endMsg{err: err} })
	}
	_ = file.Close()
	cmd.Env = append(cmd.Environ(), ErrorFileEnv+"="+file.Name())
	return hand(inTerminal(cmd), func(err error) tea.Msg {
		data, _ := os.ReadFile(file.Name())
		_ = os.Remove(file.Name())
		return endMsg{err: err, reason: strings.TrimSpace(string(data))}
	})
}

// inTerminal wraps a command that needs the real terminal, so its output stays on screen until the user goes back.
func inTerminal(cmd *exec.Cmd) *exec.Cmd {
	script := `"$@"; status=$?; printf '\npress enter to go back to the menu '; read -r _; exit $status`
	wrapped := exec.Command("sh", append([]string{"-c", script, "sh", cmd.Path}, cmd.Args[1:]...)...) //nolint:gosec,noctx // the menu runs its own binary, which the app's context ends with the program
	wrapped.Env, wrapped.Dir = cmd.Env, cmd.Dir
	return wrapped
}
