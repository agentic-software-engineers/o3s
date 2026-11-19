package tui

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// lineMsg is one line a running command printed, and whether it redraws the line before it, as a progress bar that
// ends its line in a carriage return does.
type lineMsg struct {
	text   string
	redraw bool
}

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
		lines.Split(splitLines)
		told, redraw := false, false
		// Pass every line on as it comes, a line after a carriage return redrawing the one before, until the command
		// closes its output
		for lines.Scan() {
			text, returned := strings.CutSuffix(lines.Text(), "\r")
			told = told || strings.HasPrefix(strings.TrimSpace(text), "error:")
			s <- lineMsg{text: text, redraw: redraw}
			redraw = returned
		}
		// Name output that fails to read
		if err := lines.Err(); err != nil {
			s <- lineMsg{text: "error: " + err.Error()}
		}
		s <- endMsg{err: <-done, told: told}
	}()
	return s
}

// splitLines splits output into lines at a newline, which it drops, and at a carriage return, which it keeps at the end
// of the line, so the reader knows the next line redraws it.
func splitLines(data []byte, atEOF bool) (int, []byte, error) {
	// Hand back the line before the first newline or carriage return
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		// Keep a carriage return, which marks a line that the next one redraws
		if data[i] == '\r' {
			return i + 1, data[:i+1], nil
		}
		return i + 1, data[:i], nil
	}
	// Hand back the last line of output that ends without a newline
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
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
