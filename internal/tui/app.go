package tui

import (
	"context"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hansehart/o3s/internal/menu"
)

// state is what the app shows.
type state int

const (
	// loading works out the next question or the commands.
	loading state = iota
	// asking shows a question.
	asking
	// running shows the output of commands as they run.
	running
	// done shows the output until the user goes back.
	done
)

// Config is what the app reads and runs.
type Config struct {
	// Home reads the home the menu runs in, anew for each dialog.
	Home func() menu.Home
	// Command builds the command line of a dialog's command, the binary's own.
	Command func(args []string) *exec.Cmd
	// Exec hands the terminal to a command, as tea.ExecProcess does.
	Exec func(cmd *exec.Cmd, fn tea.ExecCallback) tea.Cmd
}

// Messages the app's own commands send.
type (
	homeMsg  menu.Home
	askedMsg struct {
		q    menu.Question
		done bool
	}
	plannedMsg []menu.Command
)

// model is the whole menu: the dialog so far, what it asks, and the output of what it runs.
type model struct {
	cfg     Config
	home    menu.Home
	answers menu.Answers
	crumbs  []string
	kinds   []menu.Kind
	carry   []string
	state   state
	q       menu.Question
	field   field
	queue   []menu.Command
	out     pane
	stream  stream
	ran     int
	failed  bool
	width   int
	height  int
}

// Run shows the menu until the user leaves.
func Run(ctx context.Context, cfg Config) error {
	// Hand the terminal to a command the way bubbletea does, unless the caller does it otherwise
	if cfg.Exec == nil {
		cfg.Exec = tea.ExecProcess
	}
	_, err := tea.NewProgram(model{cfg: cfg, width: 80, height: 24}, tea.WithContext(ctx)).Run()
	return err
}

// Init reads the home for the first dialog.
func (m model) Init() tea.Cmd {
	return m.reload()
}

// reload reads the home anew.
func (m model) reload() tea.Cmd {
	read := m.cfg.Home
	return func() tea.Msg { return homeMsg(read()) }
}

// ask works out the next question of the dialog, or that it asks nothing more, away from the screen.
func (m model) ask() tea.Cmd {
	h, a := m.home, m.answers
	return func() tea.Msg {
		q, done := menu.Next(h, a)
		return askedMsg{q: q, done: done}
	}
}

// plan works out the dialog's commands, away from the screen.
func (m model) plan() tea.Cmd {
	h, a := m.home, m.answers
	return func() tea.Msg { return plannedMsg(menu.Plan(h, a)) }
}

// Update follows a message: a key, a resize, a question worked out, or a line of output.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Follow the message
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.out.size(m.width, m.paneHeight())
		return m, nil
	case homeMsg:
		m.home, m.answers, m.crumbs, m.kinds, m.carry = menu.Home(msg), nil, nil, nil, nil
		m.state = loading
		return m, m.ask()
	case askedMsg:
		return m.asked(msg)
	case plannedMsg:
		m.queue, m.failed, m.ran = msg, false, 0
		m.out = pane{}
		m.out.size(m.width, m.paneHeight())
		// Go straight back to the menu where the dialog runs nothing
		if len(m.queue) == 0 {
			return m, m.reload()
		}
		m.state = running
		return m.runNext()
	case lineMsg:
		m.out.add(string(msg))
		return m, m.stream.next()
	case endMsg:
		return m.ended(msg.err)
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	// Pass anything else to the field that asks, such as its cursor
	if m.state == asking {
		var cmd tea.Cmd
		m.field, _, cmd = m.field.Update(msg)
		return m, cmd
	}
	return m, nil
}

// asked shows the next question, or plans the commands of a dialog that asks nothing more.
func (m model) asked(msg askedMsg) (tea.Model, tea.Cmd) {
	// Plan the commands once the dialog asks nothing more
	if msg.done {
		return m, m.plan()
	}
	q := msg.q
	// Offer the answer the user stepped back from
	if m.carry != nil {
		q.Default, m.carry = m.carry, nil
	}
	m.q, m.field, m.state = q, newField(q), asking
	return m, nil
}

// key follows a key press in the state the app is in.
func (m model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Leave at once on Ctrl+C
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}
	// Follow the key in the state the app is in
	switch m.state {
	case asking:
		var r result
		var cmd tea.Cmd
		m.field, r, cmd = m.field.Update(k)
		return m.follow(r, cmd)
	case done:
		// Go back to the menu on Enter or Esc
		if k.String() == "enter" || k.String() == "esc" {
			return m, m.reload()
		}
		m.out.scroll(k)
	case running:
		m.out.scroll(k)
	case loading:
	}
	return m, nil
}

// follow takes the outcome of a key on the field: the next question, a step back, or leaving from the main menu.
func (m model) follow(r result, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	// Follow what the key did to the field
	switch r.outcome {
	case answered:
		m.answers = m.answers.With(m.q.Key, r.values)
		m.crumbs, m.kinds = append(m.crumbs, crumb(m.q, r.values)), append(m.kinds, m.q.Kind)
		m.state = loading
		return m, m.ask()
	case back:
		// Leave from the main menu
		if len(m.answers) == 0 {
			return m, tea.Quit
		}
		n := len(m.answers) - 1
		last, kind := m.answers[n], m.kinds[n]
		m.answers, m.crumbs, m.kinds = m.answers[:n], m.crumbs[:n], m.kinds[:n]
		// Carry a shown answer back into its question, and leave a hidden one out
		if kind != menu.Hidden {
			m.carry = last.Values
		}
		m.state = loading
		return m, m.ask()
	case open:
	}
	return m, cmd
}

// runNext runs the next command of the dialog, shows a note in its place, or ends once none is left.
func (m model) runNext() (tea.Model, tea.Cmd) {
	// Show every note before the next command
	for len(m.queue) > 0 && m.queue[0].Note != "" && len(m.queue[0].Args) == 0 {
		m.out.add(m.queue[0].Note)
		m.queue = m.queue[1:]
	}
	// End once every command ran, naming that they all succeeded
	if len(m.queue) == 0 {
		// Confirm a dialog whose commands all ran
		if m.ran > 0 {
			m.out.add(added.Render("✓ done"))
		}
		m.state = done
		return m, nil
	}
	c := m.queue[0]
	m.queue, m.ran = m.queue[1:], m.ran+1
	m.out.add(dim.Render("$ o3s " + strings.Join(c.Args, " ")))
	cmd := m.cfg.Command(c.Args)
	// Hand the terminal to a command that needs it, and come back when it ends
	if c.Terminal {
		return m, m.cfg.Exec(inTerminal(cmd), func(err error) tea.Msg { return endMsg{err: err} })
	}
	stdin := ""
	// Hand the value the dialog collected over stdin
	if c.Stdin != nil {
		stdin = *c.Stdin
	}
	m.stream = capture(cmd, stdin)
	return m, m.stream.next()
}

// ended takes the end of a command: the next one runs, or the dialog stops at a failure.
func (m model) ended(err error) (tea.Model, tea.Cmd) {
	// Stop at a command that fails, naming how
	if err != nil {
		m.out.add("✗ " + err.Error())
		m.queue, m.failed, m.state = nil, true, done
		return m, nil
	}
	return m.runNext()
}

// crumb is how an answer shows in the trail of the dialog.
func crumb(q menu.Question, values []string) string {
	// Keep a hidden value hidden
	if q.Kind == menu.Hidden {
		return "••••"
	}
	return strings.Join(values, ", ")
}

// paneHeight is the room the output has below the header and the trail, above the hint.
func (m model) paneHeight() int {
	return m.height - 5
}

// View draws the whole screen from the state: header, trail, body, hint.
func (m model) View() tea.View {
	top := header.Render("o3s") + "  " + dim.Render(m.home.Dir) + "\n" + strings.Join(m.crumbs, separator) + "\n\n"
	var body, hint string
	// Draw the body and the hint of the state
	switch m.state {
	case asking:
		body, hint = m.field.View(m.width), dim.Render(m.field.Hint())
	case loading:
		body = dim.Render("…")
	case running:
		body, hint = m.out.View(), dim.Render("running…")
	case done:
		body, hint = m.out.View(), dim.Render("↵/esc back to the menu · pgup/pgdn scroll")
		// Name a dialog that stopped at a failure
		if m.failed {
			hint = warning.Render("failed") + dim.Render(" · ") + hint
		}
	}
	v := tea.NewView(top + body + "\n\n" + hint)
	v.AltScreen = true
	return v
}
