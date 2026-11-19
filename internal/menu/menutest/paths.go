package menutest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/hansehart/o3s/internal/fixture"
	"github.com/hansehart/o3s/internal/menu"
)

// Path is one way through the menu: the questions it asks, the answers it takes, and the commands it ends in. A stuck
// path ends at a question that offers no answer.
type Path struct {
	Asked   []menu.Question
	Answers menu.Answers
	Plan    []menu.Command
	Stuck   bool
}

// maxDepth bounds a path, so a menu that keeps asking fails the walk instead of running forever.
const maxDepth = 30

// typed are the lines a test types for each question that takes text, tried in turn until the question's check
// takes one.
var typed = map[string][]string{
	"hosts":      {fixture.NewHost + " " + fixture.OtherWildcard},
	"portsTyped": {strconv.Itoa(fixture.OtherPort) + ", " + strconv.Itoa(fixture.Port)},
	"forTyped":   {fixture.Duration},
	"secretName": {fixture.NewSecret},
	"name":       {fixture.NewSecret},
	"store":      {fixture.AbsentStore},
	"repos":      {fixture.Repo},
	"typed":      {fixture.Repo},
}

// Walk follows the menu on a home from the main menu until it covers every answer of every question at least once,
// the edge coverage model-based testing aims for, as GraphWalker's edge_coverage(100). An answer counts once per
// dialog it is given in, so a path takes an answer already covered only where it has to go on.
func Walk(h menu.Home) ([]Path, error) {
	var out []Path
	covered := map[string]bool{}
	var walk func(p Path) error
	walk = func(p Path) error {
		q, done := menu.Next(h, p.Answers)
		// End a path once the menu asks nothing more
		if done {
			p.Plan = menu.Plan(h, p.Answers)
			out = append(out, p)
			return nil
		}
		// Turn away a menu that keeps asking
		if len(p.Answers) >= maxDepth {
			return fmt.Errorf("the menu asks beyond %d questions after %v", maxDepth, p.Answers)
		}
		answers, err := Answers(q)
		// Hand back a question the walk can't answer
		if err != nil {
			return err
		}
		p.Asked = append(slices.Clone(p.Asked), q)
		// End a path at a question that offers no answer
		if len(answers) == 0 {
			p.Stuck = true
			out = append(out, p)
			return nil
		}
		followed := false
		// Follow every answer not covered yet
		for _, values := range answers {
			e := edge(p.Answers, q.Key, values)
			// Skip an answer covered already
			if covered[e] {
				continue
			}
			covered[e], followed = true, true
			// Hand back a failure further down
			if err := walk(p.with(q.Key, values)); err != nil {
				return err
			}
		}
		// Go on with the first answer where every answer is covered already
		if !followed {
			return walk(p.with(q.Key, answers[0]))
		}
		return nil
	}
	return out, walk(Path{})
}

// with is the path with one more answer.
func (p Path) with(key string, values []string) Path {
	p.Answers = p.Answers.With(key, values)
	return p
}

// edge names an answer within the dialog it is given in: the group, the tab and the action picked before it, the
// question, and the answer.
func edge(a menu.Answers, key string, values []string) string {
	var dialog []string
	// Name the dialog by the answers that pick it
	for _, x := range a {
		// Take the group, the view's tab and the action
		switch x.Key {
		case "do", "action":
			dialog = append(dialog, x.Key+"="+strings.Join(x.Values, ","))
		case "view":
			dialog = append(dialog, "view="+x.Values[0])
		}
	}
	return strings.Join(dialog, " ") + " | " + key + "=" + strings.Join(values, ",")
}

// Answers are every answer a question takes on the screen, each the values the screen hands back.
func Answers(q menu.Question) ([][]string, error) {
	var out [][]string
	// Answer by the kind of question
	switch q.Kind {
	case menu.Select:
		// Pick every option
		for _, o := range q.Options {
			out = append(out, []string{o.Value})
		}
	case menu.Multi:
		all := make([]string, len(q.Options))
		// Mark every option alone, then all of them
		for i, o := range q.Options {
			out, all[i] = append(out, []string{o.Value}), o.Value
		}
		// Mark all of several options
		if len(all) > 1 {
			out = append(out, all)
		}
	case menu.Confirm:
		out = [][]string{{"yes"}, {"no"}}
	case menu.Hidden:
		out = [][]string{{fixture.SecretValue}}
	case menu.Input:
		line, err := typedLine(q)
		// Hand back a question without a line to type
		if err != nil {
			return nil, err
		}
		out = [][]string{{line}}
	case menu.View:
		// Answer on every tab
		for _, t := range q.Tabs {
			out = append(out, tabAnswers(t)...)
		}
	}
	return out, nil
}

// typedLine is the line a test types for a question that takes text, the first its check takes.
func typedLine(q menu.Question) (string, error) {
	lines, ok := typed[q.Key]
	// Type a proposal for a feature's option
	if strings.HasPrefix(q.Key, "option:") {
		lines, ok = []string{fixture.Proposal}, true
	}
	// Name a question no line is set for
	if !ok {
		return "", fmt.Errorf("no line to type for %q, add one to menutest's typed lines", q.Key)
	}
	// Take the first line the question's check takes
	for _, line := range lines {
		// Take a line the check takes, or any where it checks nothing
		if q.Check == nil {
			return line, nil
		}
		// Take a line the check reads
		if _, err := q.Check(line); err == nil {
			return line, nil
		}
	}
	return "", fmt.Errorf("%q's check takes none of %v", q.Key, lines)
}

// tabAnswers are every answer a view's tab takes: each row and key, the marks as they stand, each mark turned and
// none, and the first row moved one place down.
func tabAnswers(t menu.Tab) [][]string {
	var out [][]string
	// Act on every key of the tab
	for _, k := range t.Keys {
		out = append(out, []string{t.Name, k.Value})
	}
	// Hand back marks on a tab that takes them
	if t.Multi {
		return append(out, markAnswers(t)...)
	}
	// Pick every row
	for _, o := range t.Options {
		out = append(out, []string{t.Name, o.Value})
	}
	// Move the first row one place down on a tab that lets rows move
	if t.Reorder && len(t.Options) > 1 {
		order := make([]string, len(t.Options))
		// Take every row's value
		for i, o := range t.Options {
			order[i] = o.Value
		}
		order[0], order[1] = order[1], order[0]
		out = append(out, append([]string{t.Name, menu.Order, order[1]}, order...))
	}
	return out
}

// markAnswers are the marks a tab hands back: as they stand, with each row turned, and none, each in the rows' order.
func markAnswers(t menu.Tab) [][]string {
	var out [][]string
	marks := func(on func(menu.Option) bool) []string {
		values := []string{t.Name}
		// Take every row marked
		for _, o := range t.Options {
			// Take a marked row
			if on(o) {
				values = append(values, o.Value)
			}
		}
		return values
	}
	marked := func(o menu.Option) bool { return slices.Contains(t.Marked, o.Value) }
	out = append(out, marks(marked))
	// Turn every row's mark alone
	for _, turned := range t.Options {
		out = append(out, marks(func(o menu.Option) bool { return marked(o) != (o.Value == turned.Value) }))
	}
	// Hand back no marks where some stand
	if len(t.Marked) > 0 {
		out = append(out, []string{t.Name})
	}
	return out
}
