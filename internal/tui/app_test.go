package tui

import (
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hansehart/o3s/internal/menu"
)

// stripped is a view without its colors.
func stripped(s string) string {
	return ansi.Strip(s)
}

// settle runs every command the app hands back, feeding each message in, until the app is quiet, and reports whether it quit.
func settle(m model, cmd tea.Cmd) (model, bool) {
	// Run commands until none is left
	for cmd != nil {
		msg := cmd()
		// Report the app leaving
		if _, quit := msg.(tea.QuitMsg); quit {
			return m, true
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(model)
	}
	return m, false
}

// anApp is the app on a home, running each command as a shell line that echoes it, or the script given for it.
func anApp(t *testing.T, scripts map[string]string) (model, *[]string) {
	t.Helper()
	var handed []string
	m := model{width: 80, height: 24, cfg: Config{
		Home: func() menu.Home { return menu.Home{Dir: "/home/ubuntu/o3s", Exists: true} },
		Command: func(args []string) *exec.Cmd {
			line := strings.Join(args, " ")
			script, ok := scripts[line]
			// Echo a command without its own script
			if !ok {
				script = "echo ran " + line
			}
			return exec.CommandContext(t.Context(), "sh", "-c", script)
		},
		Exec: func(cmd *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
			handed = append(handed, strings.Join(cmd.Args[4:], " "))
			return func() tea.Msg { return fn(nil) }
		},
	}}
	m, _ = settle(m, m.Init())
	return m, &handed
}

// drive presses keys on the app, settling after each, and reports whether it quit.
func drive(m model, msgs ...tea.Msg) (model, bool) {
	quit := false
	// Press every key and settle
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m, quit = settle(next.(model), cmd)
		// Stop once the app left
		if quit {
			break
		}
	}
	return m, quit
}

// A dialog runs its command inside the app, shows the output until Enter, and goes back to a fresh main menu.
func TestAppRunsADialogAndGoesBack(t *testing.T) {
	m, _ := anApp(t, nil)

	ran, _ := drive(m, keys("fir", enter, "ls", enter)...)
	back, _ := drive(ran, enter)

	// Require the output on screen, the dialog done, and its success named
	if view := stripped(ran.View().Content); ran.state != done || !strings.Contains(view, "ran firewall ls") || !strings.Contains(view, "firewall / ls") || !strings.Contains(view, "done") {
		t.Errorf("after ls = state %v, view:\n%s", ran.state, view)
	}
	// Require a fresh main menu
	if back.state != asking || back.q.Key != "do" || len(back.answers) != 0 {
		t.Errorf("after enter = state %v on %q with %d answers, want the main menu", back.state, back.q.Key, len(back.answers))
	}
}

// Esc steps back one question and offers the answer given there.
func TestAppStepsBackWithTheAnswer(t *testing.T) {
	m, _ := anApp(t, nil)

	m, _ = drive(m, keys("fir", enter, "add", enter, "example.com", enter, esc)...)

	// Require the host question again, holding the host typed
	if m.q.Key != "host" || !strings.Contains(stripped(m.field.View(80)), "> example.com") {
		t.Errorf("after esc = %q, view:\n%s", m.q.Key, stripped(m.field.View(80)))
	}
	// Require the trail without the host
	if strings.Join(m.crumbs, " / ") != "firewall / add" {
		t.Errorf("trail = %q, want firewall / add", strings.Join(m.crumbs, " / "))
	}
}

// Esc on the main menu and Ctrl+C anywhere leave.
func TestAppLeaves(t *testing.T) {
	m, _ := anApp(t, nil)

	_, escQuit := drive(m, esc)
	_, ctrlQuit := drive(m, keys("fir", enter, ctrlC)...)

	// Require both to leave
	if !escQuit || !ctrlQuit {
		t.Errorf("esc quit %v, ctrl+c quit %v, want both", escQuit, ctrlQuit)
	}
}

// A failing command stops the dialog and names how it failed.
func TestAppShowsAFailure(t *testing.T) {
	m, _ := anApp(t, map[string]string{"firewall ls": "echo boom; exit 3"})

	m, _ = drive(m, keys("fir", enter, "ls", enter)...)

	view := stripped(m.View().Content)
	// Require the output, the status and the failure
	if !m.failed || !strings.Contains(view, "boom") || !strings.Contains(view, "error: exit status 3") {
		t.Errorf("failed %v, view:\n%s", m.failed, view)
	}
}

// A command that needs the real terminal gets it, and the app carries on after it.
func TestAppHandsTheTerminalToTheShell(t *testing.T) {
	m, handed := anApp(t, nil)

	m, _ = drive(m, keys("cage", enter, "shell", enter)...)

	// Require the shell handed the terminal, and the dialog done
	if len(*handed) != 1 || !strings.HasSuffix((*handed)[0], "cage shell") || m.state != done {
		t.Errorf("handed %v, state %v, want cage shell and done", *handed, m.state)
	}
}
