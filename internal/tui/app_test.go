package tui

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hansehart/o3s/internal/fixture"
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
	m := model{width: usualWidth, height: usualHeight, cfg: Config{
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

	m, _ = drive(m, keys("fir", enter, "add", enter, fixture.Host, enter, esc)...)

	// Require the host question again, holding the host typed
	if m.q.Key != "host" || !strings.Contains(stripped(m.field.View(usualWidth)), "> "+fixture.Host) {
		t.Errorf("after esc = %q, view:\n%s", m.q.Key, stripped(m.field.View(usualWidth)))
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

// A command on the real terminal that fails leaves its own reason in the menu, which outlasts the terminal it printed on.
func TestAppKeepsTheReasonOfAFailureOnTheTerminal(t *testing.T) {
	m, _ := anApp(t, nil)
	m.cfg.Exec = func(cmd *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		// Write the reason where the command is told to, as o3s does
		for _, kv := range cmd.Env {
			// Write into the file the menu named
			if file, ok := strings.CutPrefix(kv, "O3S_ERROR_FILE="); ok {
				_ = os.WriteFile(file, []byte("error: the cage needs AppArmor\n"), 0o600)
			}
		}
		return func() tea.Msg { return fn(errors.New("exit status 1")) }
	}

	m, _ = drive(m, keys("cage", enter, "shell", enter)...)

	view := stripped(m.View().Content)
	// Require the command's own reason
	if !m.failed || !strings.Contains(view, "error: the cage needs AppArmor") || strings.Contains(view, "exit status 1") {
		t.Errorf("failed %v, view:\n%s", m.failed, view)
	}
}

// A captured command that names its own error shows that error alone.
func TestAppNamesACapturedErrorOnce(t *testing.T) {
	m, _ := anApp(t, map[string]string{"firewall ls": "echo 'error: boom'; exit 3"})

	m, _ = drive(m, keys("fir", enter, "ls", enter)...)

	view := stripped(m.View().Content)
	// Require the command's own error alone
	if !m.failed || !strings.Contains(view, "error: boom") || strings.Contains(view, "exit status 3") {
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

// Every kind of question shows what it explains above it, a long line broken at its words.
func TestAppShowsTheExplanationOfEveryQuestion(t *testing.T) {
	note := "Channel (lts|latest), a release line (major or major.minor) or an exact version to install"
	// Ask every kind of question
	for _, kind := range []menu.Kind{menu.Input, menu.Select, menu.Confirm} {
		q := menu.Question{Key: "k", Kind: kind, Title: "node version", Options: []menu.Option{{Value: "lts"}}, Default: []string{"lts"}, Preview: []string{note}}
		m := model{width: narrow, height: usualHeight, state: asking, q: q, field: newField(q)}

		view := stripped(m.View().Content)

		// Require every word of the explanation, with each line within the width
		for word := range strings.FieldsSeq(note) {
			// Report a word the screen lost
			if !strings.Contains(view, word) {
				t.Errorf("kind %v lacks %q:\n%s", kind, word, view)
			}
		}
		// Require the explanation once
		if n := strings.Count(view, "Channel"); n != 1 {
			t.Errorf("kind %v shows the explanation %d times:\n%s", kind, n, view)
		}
	}
}

// A menu takes its size from the terminal's first report.
func TestAppTakesItsSizeFromTheTerminal(t *testing.T) {
	m, _ := anApp(t, nil)
	m.width, m.height = 0, 0

	next, _ := m.Update(tea.WindowSizeMsg{Width: narrow, Height: paneLines + chromeLines})
	m = next.(model)

	// Require the size the terminal reported
	if m.width != narrow || m.height != paneLines+chromeLines {
		t.Errorf("size = %dx%d, want %dx%d", m.width, m.height, narrow, paneLines+chromeLines)
	}
}
