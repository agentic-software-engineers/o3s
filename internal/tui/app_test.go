package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/menu"
)

// stripped is a view without its colors.
func stripped(s string) string {
	return ansi.Strip(s)
}

// settle runs every command the app hands back, a batch's commands each in turn, feeding each message in until the
// app is quiet, and reports whether it quit. It leaves the messages a timer sends unfed, a view's next ask for its rows
// and a spinner's next frame, which a test sends itself where it needs one.
func settle(m model, cmd tea.Cmd) (model, bool) {
	cmds := []tea.Cmd{cmd}
	// Run commands until none is left
	for len(cmds) > 0 {
		next := cmds[0]
		cmds = cmds[1:]
		// Pass over an empty command
		if next == nil {
			continue
		}
		// Follow what the command hands back
		switch msg := next().(type) {
		case tea.QuitMsg:
			return m, true
		case tea.BatchMsg:
			cmds = append(cmds, msg...)
		case refreshMsg:
		default:
			updated, more := m.Update(msg)
			m = updated.(model)
			cmds = append(cmds, more)
		}
	}
	return m, false
}

// anApp is the app on a home, running each command as a shell line that echoes it, or the script given for it.
func anApp(t *testing.T, scripts map[string]string) (model, *[]string) {
	t.Helper()
	var handed []string
	m := model{width: usualWidth, height: usualHeight, cfg: Config{
		Home: func() menu.Home { return menu.Home{Dir: fixture.HomeDir, Exists: true} },
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

// allowLine is the command line allowing the fixture host on the HTTPS port for good ends in.
var allowLine = "firewall add " + fixture.Host + " --port " + port + " --yes"

// allowing are the keys that allow the fixture host on the HTTPS port for good, from the main menu to the confirmation.
func allowing() []tea.Msg {
	return keys("fir", enter, enter, fixture.Host, enter, "1", "1", "1")
}

// A dialog runs its command inside the app, shows the output and its success, and Enter goes back to the group's view.
func TestAppRunsADialogAndGoesBackToTheGroup(t *testing.T) {
	m, _ := anApp(t, nil)

	ran, _ := drive(m, append(allowing(), enter)...)
	back, _ := drive(ran, enter)

	view := stripped(ran.View().Content)
	// Require the command line, its output, its success and the way back to the group
	for _, want := range []string{"$ o3s " + allowLine, "ran " + allowLine, "✓ done", "enter back to firewall", "esc menu"} {
		// Report a part the screen lacks
		if ran.state != done || !strings.Contains(view, want) {
			t.Errorf("after the dialog, state %v, view lacks %q:\n%s", ran.state, want, view)
		}
	}
	// Require the firewall's view again
	if back.state != asking || back.q.Key != "view" || len(back.answers) != 1 {
		t.Errorf("after enter = state %v on %q with %d answers, want the firewall's view", back.state, back.q.Key, len(back.answers))
	}
}

// Esc steps back one question and offers the answer given there.
func TestAppStepsBackWithTheAnswer(t *testing.T) {
	m, _ := anApp(t, nil)

	m, _ = drive(m, keys("fir", enter, enter, fixture.Host, enter, esc)...)

	// Require the hosts question again, holding the host typed
	if view := stripped(m.field.View(usualWidth, usualHeight)); m.q.Key != "hosts" || !strings.Contains(view, "❯ "+fixture.Host) {
		t.Errorf("after esc = %q, view:\n%s", m.q.Key, view)
	}
	// Require the trail without the host
	if strings.Join(m.crumbs(), " / ") != "firewall / Allowed / Allow a host" {
		t.Errorf("trail = %q, want firewall / Allowed / Allow a host", strings.Join(m.crumbs(), " / "))
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

// A menu opened at a group's view leaves on Esc there, and goes back to that view after a dialog.
func TestAppOpenedAtAGroupStaysThere(t *testing.T) {
	m, _ := anApp(t, nil)
	m.cfg.Start = menu.Open("firewall")
	m, _ = settle(m, m.Init())

	ran, _ := drive(m, keys(enter, fixture.Host, enter, "1", "1", "1", enter)...)
	back, _ := drive(ran, enter)
	_, quit := drive(back, esc)

	// Require the dialog run from the firewall's view
	if ran.state != done || !strings.Contains(stripped(ran.View().Content), "ran "+allowLine) {
		t.Errorf("after the dialog, state %v, view:\n%s", ran.state, stripped(ran.View().Content))
	}
	// Require the firewall's view again, and Esc there to leave
	if back.q.Key != "view" || !quit {
		t.Errorf("after enter on %q, esc quit %v, want the firewall's view and leaving", back.q.Key, quit)
	}
}

// A declined change says that nothing changed, and goes back to the group's view.
func TestAppNamesADeclinedChange(t *testing.T) {
	m, _ := anApp(t, nil)

	m, _ = drive(m, append(allowing(), typed("n")...)...)

	// Require the note and the way back, with nothing run
	if view := stripped(m.View().Content); m.state != done || !strings.Contains(view, "Nothing changed.") || strings.Contains(view, "$ o3s") {
		t.Errorf("state %v, view:\n%s", m.state, view)
	}
}

// Quiet commands that succeed show the view they changed, at the row they name, without their output.
func TestAppShowsTheViewAfterQuietCommands(t *testing.T) {
	m, _ := anApp(t, nil)
	m.cfg.Home = func() menu.Home {
		return menu.Home{Dir: fixture.HomeDir, Exists: true, Installed: []menu.Installed{{Package: fixture.Store + "/" + fixture.Feature, Tag: fixture.Major}, {Package: fixture.Store + "/" + fixture.OtherFeature, Tag: fixture.Major}}}
	}
	m, _ = settle(m, m.Init())

	m, _ = drive(m, keys("feat", enter, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}, enter)...)

	// Require the feature view again, without a result screen
	if m.state != asking || m.q.Key != "view" {
		t.Errorf("after the move, state %v on %q, want the feature view", m.state, m.q.Key)
	}
}

// A failing command stops the dialog and names how it failed.
func TestAppShowsAFailure(t *testing.T) {
	m, _ := anApp(t, map[string]string{allowLine: "echo boom; exit 3"})

	m, _ = drive(m, append(allowing(), enter)...)

	view := stripped(m.View().Content)
	// Require the output, the failure and its reason
	if !m.failed || !strings.Contains(view, "boom") || !strings.Contains(view, "error: exit status 3") || !strings.Contains(view, "✗ failed") {
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
	m, _ := anApp(t, map[string]string{allowLine: "echo 'error: boom'; exit 3"})

	m, _ = drive(m, append(allowing(), enter)...)

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

// A trail wider than the screen keeps its end in view, cut at its start.
func TestAppKeepsTheEndOfALongTrail(t *testing.T) {
	m, _ := anApp(t, nil)
	m.width = narrow

	m, _ = drive(m, keys("fir", enter, enter, fixture.Host+" "+fixture.Subdomain+" "+fixture.OtherHost, enter, "1")...)

	trail := strings.Split(stripped(m.View().Content), "\n")[1]
	// Require the trail within the width, its start cut and its end, the last answer, in view
	if ansi.StringWidth(trail) > narrow || !strings.HasPrefix(trail, "…") || !strings.HasSuffix(trail, port) {
		t.Errorf("trail = %q, want its end within %d columns", trail, narrow)
	}
}

// A command line shows as a shell takes it: a value with a space quoted, and a flag held to its value where it breaks.
func TestEchoedQuotesAndHoldsFlagsToTheirValues(t *testing.T) {
	got := echoed([]string{"firewall", "add", "bad host!", "--port", port})

	// Require the value quoted and the flag held to its value
	if want := "firewall add 'bad host!' --port" + nonBreakingSpace + port; got != want {
		t.Errorf("echoed() = %q, want %q", got, want)
	}
	// Require the flag and its value on one line however narrow the screen
	if lines := wrapped(got, len("firewall add 'bad host!' --port")); !strings.HasSuffix(lines[len(lines)-1], "--port 443") {
		t.Errorf("wrapped() = %q, want --port 443 on one line", lines)
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

// aLongConfirmation is a confirmation whose preview runs longer than a usual screen, summed up beside the question.
func aLongConfirmation() model {
	var preview []string
	// Fill the preview past a usual screen
	for i := range 30 {
		preview = append(preview, fmt.Sprintf("  + line %02d", i))
	}
	q := menu.Question{Key: "apply", Kind: menu.Confirm, Title: "Apply?", Preview: preview, Summary: "Adds 4 features with 9 hosts"}
	return model{width: usualWidth, height: usualHeight, state: asking, q: q, field: newField(q)}
}

// A confirmation keeps its summary, its question and its answers in view however long its preview runs.
func TestAppKeepsALongConfirmationInView(t *testing.T) {
	m := aLongConfirmation()

	view := stripped(m.View().Content)

	// Require the summary, the question, the answers and the way to see the rest
	for _, want := range []string{"Adds 4 features with 9 hosts", "Apply?", "Yes", "No", "pgup/pgdn scroll", "line 00"} {
		// Report a part out of view
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	// Require the screen's height to hold it all
	if n := strings.Count(view, "\n") + 1; n > usualHeight {
		t.Errorf("view holds %d lines, want at most %d:\n%s", n, usualHeight, view)
	}
}

// Paging down shows the later lines of a long preview, with the question still in view.
func TestAppPagesThroughALongPreview(t *testing.T) {
	m := aLongConfirmation()

	paged, _ := drive(m, tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgDown})

	view := stripped(paged.View().Content)
	// Require the last lines and the question in view
	if !strings.Contains(view, "line 29") || !strings.Contains(view, "Apply?") {
		t.Errorf("after pgdn, view:\n%s", view)
	}
}

// A preview that fits shows whole, without the way to scroll it.
func TestAppShowsAShortPreviewWhole(t *testing.T) {
	q := menu.Question{Key: "apply", Kind: menu.Confirm, Title: "Apply?", Preview: []string{"  + line 00", "  + line 01"}, Summary: "Adds 1 feature"}
	m := model{width: usualWidth, height: usualHeight, state: asking, q: q, field: newField(q)}

	view := stripped(m.View().Content)

	// Require every line and the summary, the whole preview in view
	if !strings.Contains(view, "line 01") || !strings.Contains(view, "Adds 1 feature") || strings.Contains(view, "pgup/pgdn") {
		t.Errorf("view:\n%s", view)
	}
}

// A command that redraws its line, as a progress bar does, shows the line's last state alone, the lines around it
// kept.
func TestAppRedrawsALineTheCommandRedraws(t *testing.T) {
	m, _ := anApp(t, map[string]string{allowLine: `printf 'before\n10%%\r50%%\r100%%\nafter\n'`})

	m, _ = drive(m, append(allowing(), enter)...)

	var trimmed []string
	// Take every line without the padding the pane fills it to the width with
	for line := range strings.SplitSeq(stripped(m.View().Content), "\n") {
		trimmed = append(trimmed, strings.TrimRight(line, " "))
	}
	view := strings.Join(trimmed, "\n")
	// Require the bar's last state between the lines around it, its earlier states gone
	if !strings.Contains(view, "before\n100%\nafter") || strings.Contains(view, "50%") {
		t.Errorf("view:\n%s", view)
	}
}

// slowStore is a store whose catalog comes in once it is told to, offering one feature.
type slowStore struct{ ready *bool }

// Catalog lists the store's one feature.
func (s slowStore) Catalog(string) ([]menu.Offer, error) {
	return []menu.Offer{{ID: fixture.Feature}}, nil
}

// Peek lists the feature once the catalog came in.
func (s slowStore) Peek(store string) ([]menu.Offer, bool, error) {
	catalog, err := s.Catalog(store)
	return catalog, *s.ready, err
}

// Add shows nothing a feature brings.
func (slowStore) Add(string) (menu.Preview, error) { return menu.Preview{}, nil }

// Remove shows nothing that leaves.
func (slowStore) Remove(string) (menu.Preview, error) { return menu.Preview{}, nil }

// Updates lists nothing newer.
func (slowStore) Updates() ([]menu.Update, error) { return nil, nil }

// A view waiting for rows asks again at the tab the user is on, and shows the rows once they come in.
func TestAppFillsInAViewAsItsRowsComeIn(t *testing.T) {
	ready := false
	m, _ := anApp(t, nil)
	m.cfg.Home = func() menu.Home {
		return menu.Home{Dir: fixture.HomeDir, Exists: true, Stores: []string{fixture.Store}, Lookup: slowStore{ready: &ready}}
	}
	m, _ = settle(m, m.Init())
	m, _ = drive(m, keys("feat", enter, right)...)

	waiting := stripped(m.View().Content)
	ready = true
	next, cmd := m.Update(refreshMsg{})
	m, _ = settle(next.(model), cmd)

	// Require the loading line while the store answers
	if !strings.Contains(waiting, "Loading the stores' features…") {
		t.Errorf("waiting view:\n%s", waiting)
	}
	// Require the feature on the Discover tab the user stayed on
	if view := stripped(m.View().Content); !strings.Contains(view, fixture.Feature) || !strings.Contains(view, "Every feature the stores offer") {
		t.Errorf("view after the store answered:\n%s", view)
	}
}

// Commands that replace the binary end the menu once they succeed, naming the group whose view the new binary opens
// at, and a failing one keeps the menu with its failure.
func TestAppLeavesForTheNewBinary(t *testing.T) {
	updating := func(script string) (model, bool) {
		m, _ := anApp(t, map[string]string{"update": script})
		m.cfg.Home = func() menu.Home { return menu.Home{Dir: fixture.HomeDir, Exists: true} }
		m.answers = menu.Answers{}.With("do", []string{"update"})
		next, cmd := m.Update(plannedMsg{{Args: []string{"update"}, Restart: true}})
		return settle(next.(model), cmd)
	}

	m, quit := updating("true")

	// Require the app gone, opening the update's view again
	if !quit || !cmp.Equal([]string{"update"}, m.reopen) {
		t.Errorf("quit %v, reopen %v, want the update's view", quit, m.reopen)
	}

	m, quit = updating("exit 3")

	// Require the failure in the menu, and nothing to open again
	if quit || m.reopen != nil || !strings.Contains(stripped(m.View().Content), "✗ failed") {
		t.Errorf("quit %v, reopen %v, view:\n%s", quit, m.reopen, stripped(m.View().Content))
	}
}

// A menu resumed at a group's view keeps the main menu behind it, so Esc steps back instead of leaving.
func TestAppResumesWithTheMainMenuBehind(t *testing.T) {
	m, _ := anApp(t, nil)
	m.cfg.Resume = menu.Open("firewall")
	m.resume = m.cfg.Resume
	m, _ = settle(m, m.Init())

	back, quit := drive(m, esc)

	// Require the group's view first, then the main menu on Esc
	if m.q.Key != "view" || quit || back.q.Key != "do" {
		t.Errorf("opened on %q, after esc quit %v on %q, want the view then the main menu", m.q.Key, quit, back.q.Key)
	}
}

// A change opens the last view the dialog answered again, at its row: an option's own view after an edit, and the
// group's view after a new order.
func TestAppReopensTheViewAChangeCameFrom(t *testing.T) {
	pkg := fixture.Store + "/" + fixture.Feature
	tests := []struct {
		name    string
		answers menu.Answers
		kinds   []menu.Kind
		group   menu.Answers
		place   []string
	}{
		{
			"an option's edit",
			menu.Answers{}.With("do", []string{"feature"}).With("view", []string{"Installed", pkg}).With("action", []string{"configure"}).With("options", []string{"Options", "debug"}).With("option:debug", []string{"true"}),
			[]menu.Kind{menu.Select, menu.View, menu.Select, menu.View, menu.Select},
			menu.Answers{}.With("do", []string{"feature"}).With("view", []string{"Installed", pkg}).With("action", []string{"configure"}),
			[]string{"Options", "debug"},
		},
		{
			"a group's action",
			menu.Answers{}.With("do", []string{"cage"}).With("action", []string{"stop"}),
			[]menu.Kind{menu.Select, menu.Select},
			menu.Answers{}.With("do", []string{"cage"}),
			[]string{"stop"},
		},
		{
			"a new order",
			menu.Answers{}.With("do", []string{"feature"}).With("view", []string{"Installed", menu.Order, pkg, pkg}),
			[]menu.Kind{menu.Select, menu.View},
			menu.Answers{}.With("do", []string{"feature"}),
			[]string{"Installed", pkg},
		},
	}
	// Check every change
	for _, tt := range tests {
		m := model{answers: tt.answers}
		// Take every kind as the step the dialog took
		for _, k := range tt.kinds {
			m.steps = append(m.steps, step{kind: k})
		}

		group, place := m.group(), m.place()

		// Require the view and the row the change came from
		if !cmp.Equal(tt.group, group) || !cmp.Equal(tt.place, place) {
			t.Errorf("%s: group %v at %v, want %v at %v", tt.name, group, place, tt.group, tt.place)
		}
	}
}

// A tab of marks shows in the trail by what the marks change against the ones it started with, and by how many are
// marked where it started with none or they stay as they started.
func TestCrumbNamesWhatMarksChange(t *testing.T) {
	tool, other := fixture.Store+"/"+fixture.Feature, fixture.Store+"/"+fixture.OtherFeature
	rows := []menu.Option{{Value: tool}, {Value: other}, {Value: fixture.ThirdFeature}}
	tests := []struct {
		marked []string
		values []string
		want   string
	}{
		{[]string{tool}, []string{tool, other}, "Discover / marks 1"},
		{[]string{tool, other}, []string{fixture.ThirdFeature}, "Discover / marks 1, unmarks 2"},
		{[]string{tool, other}, []string{tool, other}, "Discover / 2 marked"},
		{nil, []string{tool, other}, "Discover / 2 marked"},
	}
	// Check every change of marks
	for _, tt := range tests {
		q := menu.Question{Kind: menu.View, Tabs: []menu.Tab{{Name: "Discover", Multi: true, Options: rows, Marked: tt.marked}, {Name: "Stores"}}}

		got := crumb(q, append([]string{"Discover"}, tt.values...))

		// Report a crumb that names the change otherwise
		if got != tt.want {
			t.Errorf("crumb(%v from %v) = %q, want %q", tt.values, tt.marked, got, tt.want)
		}
	}
}

// A typed answer shows in the trail in place of the choice that asked for it, and stepping back shows the choice again.
func TestTrailShowsATypedAnswerInPlaceOfItsChoice(t *testing.T) {
	m, _ := anApp(t, nil)
	m, _ = drive(m, keys("fir", enter, enter, fixture.Host, enter, "1", "5", fixture.Duration, enter)...)

	typed := strings.Join(m.crumbs(), " / ")
	back, _ := drive(m, esc)

	// Require the duration in place of its choice, and the choice back on a step back
	if want := "firewall / Allowed / Allow a host / " + fixture.Host + " / " + port + " / " + fixture.Duration; typed != want {
		t.Errorf("trail = %q, want %q", typed, want)
	}
	// Require the choice in the trail once the step back leaves the typed answer
	if got, want := strings.Join(back.crumbs(), " / "), "firewall / Allowed / Allow a host / "+fixture.Host+" / "+port+" / Type a duration"; got != want {
		t.Errorf("trail after esc = %q, want %q", got, want)
	}
}
