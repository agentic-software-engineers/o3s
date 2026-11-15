package tui

import (
	"fmt"
	"strings"

	"github.com/hansehart/o3s/internal/menu"
)

// rows is how many options a list shows at once.
const rows = 12

// list is the filter, the cursor and the window a choice and a multi-select share.
type list struct {
	q      menu.Question
	filter string
	cursor int
}

// shown lists the options the filter keeps.
func (l list) shown() []menu.Option {
	return matching(l.q.Options, l.filter)
}

// move follows the keys that move the cursor and edit the filter, and leaves any other key alone.
func (l *list) move(key, text string) {
	n := len(l.shown())
	// Follow the key the user pressed
	switch key {
	case "up", "shift+tab":
		l.cursor = max(l.cursor-1, 0)
	case "down":
		l.cursor = min(l.cursor+1, max(n-1, 0))
	case "backspace":
		// Drop the last character of the filter
		if r := []rune(l.filter); len(r) > 0 {
			l.filter, l.cursor = string(r[:len(r)-1]), 0
		}
	default:
		// Take a typed character into the filter
		if text != "" && text != " " {
			l.filter, l.cursor = l.filter+text, 0
		}
	}
}

// highlighted hands back the option under the cursor, and whether there is one.
func (l list) highlighted() (menu.Option, bool) {
	shown := l.shown()
	// Report an empty list
	if l.cursor >= len(shown) {
		return menu.Option{}, false
	}
	return shown[l.cursor], true
}

// view draws the title, the filter once typed, and the options in reach of the cursor, each with its prefix.
func (l list) view(prefix func(menu.Option) string) string {
	lines := []string{title.Render(l.q.Title)}
	// Show the filter once the user types
	if l.filter != "" {
		lines = append(lines, dim.Render("/ ")+l.filter)
	}
	shown := l.shown()
	width := 0
	// Measure the longest option, so the notes line up
	for _, o := range shown {
		width = max(width, len(o.Value))
	}
	start := max(l.cursor-rows+1, 0)
	// Draw the options in reach of the cursor
	for i := start; i < min(start+rows, len(shown)); i++ {
		o := shown[i]
		row := "  " + prefix(o) + fmt.Sprintf("%-*s", width, o.Value)
		// Put the note beside its option
		if o.Note != "" {
			row += "   " + dim.Render(o.Note)
		}
		// Point at the highlighted option
		if i == l.cursor {
			row = pointer.Render(">") + row[1:]
		}
		lines = append(lines, row)
	}
	// Name the options below the window
	if rest := len(shown) - start - rows; rest > 0 {
		lines = append(lines, dim.Render(fmt.Sprintf("  (%d more)", rest)))
	}
	// Name a filter that matches nothing
	if len(shown) == 0 {
		lines = append(lines, dim.Render("  no match"))
	}
	return strings.Join(lines, "\n")
}

// containsFold reports whether a value contains a filter, ignoring case.
func containsFold(value, filter string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(filter))
}
