package tui

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

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
	// Start are the answers the menu opens with, such as the group whose view a command named.
	Start menu.Answers
	// Resume are the answers of a view the menu opens at with the main menu behind it, as the view a binary that
	// replaced itself was at.
	Resume menu.Answers
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
	steps   []step
	carry   []string
	state   state
	q       menu.Question
	field   field
	// previewTop is the first line of a long preview in view.
	previewTop int
	queue      []menu.Command
	// resume are the answers the next reading of the home opens at, such as the view a change came from.
	resume menu.Answers
	// quiet marks commands whose success shows as the view they changed.
	quiet bool
	// restart marks commands that replace the binary the menu runs, and reopen is the group whose view the new binary
	// opens at once they succeeded.
	restart bool
	reopen  []string
	out     pane
	stream  stream
	ran     int
	failed  bool
	width   int
	height  int
}

// Run shows the menu until the user leaves, or until commands replaced the binary it runs, where it hands back the path
// of the group whose view the new binary opens at.
func Run(ctx context.Context, cfg Config) ([]string, error) {
	// Hand the terminal to a command by default, unless the caller does it otherwise
	if cfg.Exec == nil {
		cfg.Exec = tea.ExecProcess
	}
	final, err := tea.NewProgram(model{cfg: cfg, resume: cfg.Resume}, tea.WithContext(ctx)).Run()
	m, _ := final.(model)
	return m.reopen, err
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
		start, steps := m.cfg.Start, m.steps
		// Open at the view a change came from, where it asks to
		if m.resume != nil {
			start, m.resume = m.resume, nil
		}
		m.home, m.answers, m.steps = menu.Home(msg), nil, nil
		// Open at the answers the menu starts with, each kept as the dialog gave it where it gave it before
		for i, a := range start {
			s := step{crumb: strings.Join(a.Values, ", "), kind: menu.Select}
			// Keep the step the dialog took for an answer it gave before
			if i < len(steps) {
				s = steps[i]
			}
			m.answers, m.steps = m.answers.With(a.Key, a.Values), append(m.steps, s)
		}
		m.state = loading
		return m, m.ask()
	case askedMsg:
		return m.asked(msg)
	case refreshMsg:
		return m.refreshed()
	case plannedMsg:
		m.queue, m.failed, m.ran, m.quiet, m.restart = msg, false, 0, len(msg) > 0, false
		m.out = pane{}
		m.out.size(m.width, m.paneHeight())
		// Go straight back to the group's view where the dialog runs nothing
		if len(m.queue) == 0 {
			m.resume, m.carry = m.group(), m.place()
			return m, m.reload()
		}
		// Show the output of any command that is not quiet, and restart after one that replaces the binary
		for _, c := range m.queue {
			m.quiet = m.quiet && c.Quiet
			m.restart = m.restart || c.Restart
		}
		m.state = running
		return m.runNext()
	case lineMsg:
		// Redraw the last line where the command redraws it, and add a line otherwise
		if msg.redraw {
			m.out.redraw(msg.text)
		} else {
			m.out.add(msg.text)
		}
		return m, m.stream.next()
	case endMsg:
		return m.ended(msg)
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
	m.q, m.field, m.state, m.previewTop = q, newField(q), asking, 0
	// Ask again for the rows still on their way
	if q.Pending() {
		return m, tea.Tick(refreshEvery, func(time.Time) tea.Msg { return refreshMsg{} })
	}
	return m, nil
}

// refreshEvery is how often a view asks again for rows still on their way.
const refreshEvery = 300 * time.Millisecond

// refreshMsg asks a view still waiting for rows to ask again.
type refreshMsg struct{}

// refreshed asks the view in front of the user again, at the tab and the row the user is on, so the rows that came in
// show where they belong.
func (m model) refreshed() (tea.Model, tea.Cmd) {
	v, isView := m.field.(view)
	// Leave a screen that moved on from the view
	if m.state != asking || !isView {
		return m, nil
	}
	// Ask again later while the user holds a row, which a new view would put back
	if v.holding() {
		return m, tea.Tick(refreshEvery, func(time.Time) tea.Msg { return refreshMsg{} })
	}
	m.carry = v.place()
	h, a := m.home, m.answers
	return m, func() tea.Msg {
		q, done := menu.Next(h, a)
		return askedMsg{q: q, done: done}
	}
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
		// Page through a preview longer than its room, the question staying in view
		if (k.String() == "pgup" || k.String() == "pgdown") && m.previewOverflows() {
			step := m.previewRoom()
			// Page back where the key asks it
			if k.String() == "pgup" {
				step = -step
			}
			m.previewTop = min(max(m.previewTop+step, 0), len(m.previewLines())-m.previewRoom())
			return m, nil
		}
		var r result
		var cmd tea.Cmd
		m.field, r, cmd = m.field.Update(k)
		return m.follow(r, cmd)
	case done:
		// Go back to the group's view on Enter
		if k.String() == "enter" {
			m.resume, m.carry = m.group(), m.place()
			return m, m.reload()
		}
		// Leave the menu opened at a group on Esc, and go back to the main menu otherwise
		if k.String() == "esc" {
			// Leave a menu that opened at a group's view
			if len(m.cfg.Start) > 0 {
				return m, tea.Quit
			}
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
		m.steps = append(m.steps, step{crumb: crumb(m.q, r.values), kind: m.q.Kind, completes: m.q.Completes})
		m.state = loading
		return m, m.ask()
	case back:
		// Leave from the screen the menu opened at
		if len(m.answers) <= len(m.cfg.Start) {
			return m, tea.Quit
		}
		n := len(m.answers) - 1
		last, kind := m.answers[n], m.steps[n].kind
		m.answers, m.steps = m.answers[:n], m.steps[:n]
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
		// Leave for the new binary, which opens at the group's view, where the commands replaced this one
		if m.restart {
			m.reopen = m.path()
			return m, tea.Quit
		}
		// Go straight back to the view that quiet commands changed
		if m.quiet {
			m.resume, m.carry = m.group(), m.place()
			return m, m.reload()
		}
		// Confirm a dialog whose commands all ran
		if m.ran > 0 {
			m.out.add("")
			m.out.add(added.Render("✓ done"))
		}
		m.state = done
		return m, nil
	}
	c := m.queue[0]
	m.queue, m.ran = m.queue[1:], m.ran+1
	m.out.add(dim.Render("$ o3s " + echoed(c.Args)))
	cmd := m.cfg.Command(c.Args)
	// Hand the terminal to a command that needs it, and come back when it ends
	if c.Terminal {
		return m, inTerminalLeaving(cmd, m.cfg.Exec)
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
func (m model) ended(end endMsg) (tea.Model, tea.Cmd) {
	// Run the next command after one that succeeds
	if end.err == nil {
		return m.runNext()
	}
	// Name each failure once, in the command's own words where it left any
	switch {
	case end.reason != "":
		m.out.add(end.reason)
	case !end.told:
		m.out.add("error: " + end.err.Error())
	}
	m.out.add("")
	m.out.add(warning.Render("✗ failed"))
	m.queue, m.failed, m.state = nil, true, done
	return m, nil
}

// path is the group the dialog runs in, as the menu opens at it, and none at the main menu.
func (m model) path() []string {
	g := m.group()
	// Name no group at the main menu
	if len(g) == 0 || len(g[0].Values) == 0 {
		return []string{}
	}
	return g[0].Values[:1]
}

// group is the answers that open the view a change came from: the last view the dialog answered, else the group's
// own view, which the answers the menu started with open where they name the same group, and none at the main menu.
func (m model) group() menu.Answers {
	// Open the main menu where the dialog names no group
	if len(m.answers) == 0 || m.answers[0].Key != "do" {
		return nil
	}
	// Open the last view the dialog answered, past the group's own
	if i := m.lastView(); i > 1 {
		return slices.Clone(m.answers[:i])
	}
	// Open where the menu started where it started at this group
	if len(m.cfg.Start) > 0 && m.cfg.Start[0].Values[0] == m.answers[0].Values[0] {
		return m.cfg.Start
	}
	return m.answers[:1]
}

// place is where the view a change came from opens again: the tab the user picked in and the row, the one a new
// order names.
func (m model) place() []string {
	i := m.lastView()
	// Name no place where the dialog answered no view
	if i < 0 || len(m.answers[i].Values) == 0 {
		return nil
	}
	v := m.answers[i].Values
	// Read the row a new order names
	if len(v) > 2 && v[1] == menu.Order {
		return []string{v[0], v[2]}
	}
	return v[:min(len(v), 2)]
}

// lastView is where the last answer a view took stands among the answers, and -1 where no view took one.
func (m model) lastView() int {
	// Look back from the last answer for one a view took
	for i, s := range slices.Backward(m.steps) {
		// Take an answer a view took
		if s.kind == menu.View {
			return i
		}
	}
	return -1
}

// echoed writes a command line the way a shell takes it: a value with a space or a sign the shell reads quoted, and a
// flag held to the value that follows it, so a broken line keeps the two together.
func echoed(args []string) string {
	parts := make([]string, len(args))
	// Quote every value the shell would split or read
	for i, a := range args {
		parts[i] = a
		// Quote a value that holds a space or a sign the shell reads
		if a == "" || strings.ContainsAny(a, " \t'\"$&|;<>()*?!`\\#~") {
			parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	var b strings.Builder
	previous := ""
	// Join the parts, holding a flag to its value
	for i, a := range args {
		// Join the part to the one before it
		if i > 0 {
			sep := " "
			// Hold a flag to the value that follows it
			if strings.HasPrefix(previous, "--") && !strings.HasPrefix(a, "-") {
				sep = nonBreakingSpace
			}
			b.WriteString(sep)
		}
		b.WriteString(parts[i])
		previous = a
	}
	return b.String()
}

// crumb is how an answer shows in the trail of the dialog: a hidden value hidden, a view's tab with the row picked,
// and a row by its label.
func crumb(q menu.Question, values []string) string {
	// Show what the user picked by how it shows
	switch {
	case q.Kind == menu.Hidden:
		return "***"
	case q.Kind == menu.View && len(values) > 0:
		i := slices.IndexFunc(q.Tabs, func(t menu.Tab) bool { return t.Name == values[0] })
		parts := []string{}
		// Name the tab where the view has several
		if len(q.Tabs) > 1 {
			parts = append(parts, values[0])
		}
		// Name a new order, the row or the key picked on a tab that takes one, and how many on a tab that takes marks
		switch {
		case len(values) > 1 && values[1] == menu.Order:
			parts = append(parts, "order")
		case i >= 0 && q.Tabs[i].Multi:
			parts = append(parts, marksChange(q.Tabs[i].Marked, values[1:]))
		case i >= 0 && len(values) > 1:
			parts = append(parts, label(append(slices.Clone(q.Tabs[i].Options), keyOptions(q.Tabs[i])...), values[1]))
		}
		return strings.Join(parts, " / ")
	}
	shown := make([]string, len(values))
	// Show every value by its label
	for j, v := range values {
		shown[j] = label(q.Options, v)
	}
	return strings.Join(shown, ", ")
}

// marksChange names what the marks a tab hands back change against the ones it started with, and how many are marked
// where it started with none or they stay as they started.
func marksChange(before, after []string) string {
	// Count the marks where the tab started with none
	if len(before) == 0 {
		return fmt.Sprintf("%d marked", len(after))
	}
	var parts []string
	// Name the rows marked and unmarked
	for _, c := range []struct {
		n    int
		verb string
	}{{countMissing(after, before), "marks"}, {countMissing(before, after), "unmarks"}} {
		// Name a change that happened
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", c.verb, c.n))
		}
	}
	// Count the marks where they stay as they started
	if len(parts) == 0 {
		return fmt.Sprintf("%d marked", len(after))
	}
	return strings.Join(parts, ", ")
}

// countMissing counts the values of one list the other lacks.
func countMissing(values, from []string) int {
	n := 0
	// Count every value the other list lacks
	for _, v := range values {
		// Count a value the other list lacks
		if !slices.Contains(from, v) {
			n++
		}
	}
	return n
}

// keyOptions are the keys a tab acts on as options, each shown by what it does, so a key's answer reads as its help.
func keyOptions(t menu.Tab) []menu.Option {
	out := make([]menu.Option, len(t.Keys))
	// Take every key by its help
	for i, k := range t.Keys {
		out[i] = menu.Option{Value: k.Value, Label: k.Help}
	}
	return out
}

// label is how a value shows among options: its label without the sign and the dots of a row that leads on, or the value
// itself.
func label(options []menu.Option, value string) string {
	i := slices.IndexFunc(options, func(o menu.Option) bool { return o.Value == value })
	// Show a value no option holds as it is
	if i < 0 {
		return value
	}
	return strings.TrimSuffix(strings.TrimPrefix(options[i].Shown(), "+ "), "…")
}

// paneHeight is the room the output has below the header and the trail, above the footer.
func (m model) paneHeight() int {
	return m.height - chromeLines
}

// previewLines are the lines a question shows above it, broken to the width.
func (m model) previewLines() []string {
	var out []string
	// Break every line of the preview
	for _, line := range m.q.Preview {
		out = append(out, wrapped(colored(line), m.width)...)
	}
	return out
}

// summaryLines are the summary of a question, broken to the width.
func (m model) summaryLines() []string {
	// Show a summary only where the question holds one
	if m.q.Summary == "" {
		return nil
	}
	return wrapped(m.q.Summary, m.width)
}

// fieldView draws the field in the room the screen leaves it below the summary and the start of a preview.
func (m model) fieldView() string {
	room := m.height - chromeLines - len(m.summaryLines())
	// Keep room for the start of a preview and the gap below it
	if len(m.q.Preview) > 0 {
		room -= min(len(m.previewLines()), minPaneLines) + 1
	}
	return m.field.View(m.width, max(room, minPaneLines))
}

// previewRoom is how many lines the preview gets beside the summary, the question and its answers.
func (m model) previewRoom() int {
	field := strings.Count(m.fieldView(), "\n") + 1
	return max(m.height-chromeLines-field-len(m.summaryLines())-1, minPaneLines)
}

// previewOverflows reports whether a question's preview runs longer than its room.
func (m model) previewOverflows() bool {
	return len(m.previewLines()) > m.previewRoom()
}

// question draws the preview in the room it has, then the summary, the question and its answers, which stay in view.
func (m model) question() string {
	lines := m.previewLines()
	// Show the part of a long preview the user paged to
	if m.previewOverflows() {
		top := min(m.previewTop, len(lines)-m.previewRoom())
		lines = lines[top : top+m.previewRoom()]
	}
	var out []string
	// Leave a gap between the preview and what stays in view
	if len(lines) > 0 {
		out = append(out, lines...)
		out = append(out, "")
	}
	out = append(out, m.summaryLines()...)
	return strings.Join(append(out, m.fieldView()), "\n")
}

// step is how the dialog took one answer: its crumb in the trail, the kind of question it answered, and the earlier
// question that answer finishes, where it finishes one.
type step struct {
	crumb     string
	kind      menu.Kind
	completes string
}

// crumbs are the crumbs of the trail, each answer once, an answer a later one finishes left out for the one that
// finishes it.
func (m model) crumbs() []string {
	var out []string
	// Take every step a later step leaves standing
	for i, s := range m.steps {
		// Leave out a step a later answer finishes
		if slices.ContainsFunc(m.steps[i+1:], func(later step) bool { return later.completes == m.answers[i].Key }) {
			continue
		}
		out = append(out, s.crumb)
	}
	return out
}

// trail is the path of the dialog so far, cut at its start where it runs wider than the screen, so its end stays in
// view.
func (m model) trail() string {
	line := strings.Join(m.crumbs(), separator)
	// Keep the end of a trail wider than the screen
	if over := ansi.StringWidth(line) - m.width; m.width > 0 && over > 0 {
		return ansi.TruncateLeft(line, over+1, "…")
	}
	return line
}

// keys are the keys the screen takes now, as the footer names them.
func (m model) keys() []key.Binding {
	// Name the keys of the state the app is in
	switch m.state {
	case asking:
		keys := m.field.Keys()
		// Name Esc as leaving on the screen the menu opened at
		if len(m.answers) <= len(m.cfg.Start) {
			// Swap the step back for leaving
			for i, k := range keys {
				// Take the binding of Esc
				if k.Help().Key == "esc" && k.Help().Desc == "back" {
					keys[i] = keyQuit
				}
			}
		}
		// Name the keys that page a long preview
		if m.previewOverflows() {
			keys = append([]key.Binding{bind("pgup/pgdn", "scroll", "pgup", "pgdown")}, keys...)
		}
		return keys
	case running:
		// Name scrolling where the output runs longer than the screen
		if m.out.overflows() {
			return []key.Binding{keyScroll}
		}
		return nil
	case done:
		var keys []key.Binding
		// Name scrolling where the output runs longer than the screen
		if m.out.overflows() {
			keys = append(keys, keyScroll)
		}
		back, esc := bind("enter", "back to the menu", "enter"), keyQuit
		// Name the group Enter goes back to
		if g := m.group(); len(g) > 0 {
			back = bind("enter", "back to "+g[0].Values[0], "enter")
		}
		// Name the main menu Esc goes back to where the menu started there
		if len(m.cfg.Start) == 0 {
			esc = keyMenu
		}
		return append(keys, back, esc)
	case loading:
	}
	return nil
}

// View draws the whole screen from the state: header, trail, body, footer.
func (m model) View() tea.View {
	top := header.Render("o3s") + "  " + dim.Render(m.home.Dir) + "\n" + m.trail() + "\n\n"
	var body string
	// Draw the body of the state
	switch m.state {
	case asking:
		body = m.question()
	case loading:
		body = dim.Render("…")
	case running, done:
		body = m.out.View()
	}
	v := tea.NewView(top + body + "\n\n" + footer(m.width, m.keys()))
	v.AltScreen = true
	return v
}
