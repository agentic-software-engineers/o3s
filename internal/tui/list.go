package tui

import (
	"fmt"
	"maps"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/hansehart/o3s/internal/menu"
)

// rows is how many options a list shows at once where the screen leaves it no height of its own.
const rows = 12

// list is the options, the filter, the cursor and the marks that a choice, a multi-select and a view's tab share.
type list struct {
	options []menu.Option
	// header labels the rows' label and each of their columns.
	header   []string
	filter   string
	cursor   int
	numbered bool
	// marks are the options marked, and nil for a list that takes one.
	marks map[string]bool
}

// shown lists the options the filter keeps, in their order.
func (l list) shown() []menu.Option {
	var out []menu.Option
	// Keep every option that matches
	for _, o := range l.options {
		// Take an option whose label or value contains the filter
		if containsFold(o.Shown(), l.filter) || containsFold(o.Value, l.filter) {
			out = append(out, o)
		}
	}
	return out
}

// move follows the keys that move the cursor and edit the filter, typed characters going into the filter where typing
// filters.
func (l *list) move(key, text string, typing bool) {
	n := len(l.shown())
	// Follow the key the user pressed
	switch key {
	case "up":
		l.cursor = max(l.cursor-1, 0)
	case "down":
		l.cursor = min(l.cursor+1, max(n-1, 0))
	case "backspace":
		// Drop the last character of the filter
		if r := []rune(l.filter); typing && len(r) > 0 {
			l.filter, l.cursor = string(r[:len(r)-1]), 0
		}
	default:
		// Take a typed character into the filter
		if typing && text != "" && text != " " {
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

// picked hands back the option a digit numbers, and whether it numbers one.
func (l list) picked(text string) (menu.Option, bool) {
	shown := l.shown()
	// Read the digit as the number of an option
	if !l.numbered || len(text) != 1 || text[0] < '1' || text[0] > '9' {
		return menu.Option{}, false
	}
	i := int(text[0] - '1')
	// Report a digit beyond the options
	if i >= len(shown) {
		return menu.Option{}, false
	}
	return shown[i], true
}

// toggle marks or unmarks the highlighted option.
func (l *list) toggle() {
	o, ok := l.highlighted()
	// Leave an empty list as it is
	if !ok {
		return
	}
	marks := make(map[string]bool, len(l.marks)+1)
	maps.Copy(marks, l.marks)
	marks[o.Value] = !marks[o.Value]
	l.marks = marks
}

// marked hands back the marked options in their order.
func (l list) marked() []string {
	var out []string
	// Take every marked option in order
	for _, o := range l.options {
		// Take an option the user marked
		if l.marks[o.Value] {
			out = append(out, o.Value)
		}
	}
	return out
}

// view draws the options in reach of the cursor, each on a line cut to the width with its columns lined up and the
// line that says what it is below it, within the height given.
func (l list) view(width, height int) []string {
	shown := l.shown()
	// Name a filter that matches nothing
	if len(shown) == 0 {
		return []string{dim.Render("  No match for \"" + l.filter + "\"")}
	}
	label, columns := widths(shown, l.header)
	per := 1
	// Give every option a second line where any says what it is
	for _, o := range shown {
		// Take the extra line of an option with a detail
		if o.Detail != "" {
			per = 2
			break
		}
	}
	var lines []string
	// Label the columns above the rows
	if len(l.header) > 0 {
		lines = append(lines, cut(dim.Render(l.heading(label, columns)), width))
		height--
	}
	fit := len(shown)
	// Fit the options into the height, keeping a line for the ones below, and into the usual rows without a height
	switch {
	case height > 0 && fit*per > height:
		fit = max((height-1)/per, 1)
	case height <= 0 && fit > rows:
		fit = rows
	}
	start := max(l.cursor-fit+1, 0)
	// Draw the options in reach of the cursor
	for i := start; i < min(start+fit, len(shown)); i++ {
		lines = append(lines, l.row(shown[i], i, i == l.cursor, label, columns, width)...)
	}
	// Name the options below the window
	if rest := len(shown) - start - fit; rest > 0 {
		lines = append(lines, dim.Render(fmt.Sprintf("  ↓ %d more", rest)))
	}
	return lines
}

// row draws one option: the pointer where the cursor is, its number, its mark, its label and columns lined up, its
// note, and its detail below.
func (l list) row(o menu.Option, i int, current bool, label int, columns []int, width int) []string {
	var b strings.Builder
	b.WriteString("  ")
	// Point at the highlighted option
	if current {
		b.Reset()
		b.WriteString(pointer.Render("❯ "))
	}
	// Number an option a digit picks
	if l.numbered && i < 9 {
		fmt.Fprintf(&b, "%d. ", i+1)
	}
	// Show the mark of an option the user may mark
	if l.marks != nil {
		box := "[ ] "
		// Fill the box of a marked option
		if l.marks[o.Value] {
			box = marked.Render("[x]") + " "
		}
		b.WriteString(box)
	}
	b.WriteString(pad(o.Shown(), label))
	// Line every column up with the same column of the other rows
	for c, col := range o.Columns {
		b.WriteString("   ")
		b.WriteString(pad(col, columns[c]))
	}
	// Put the note beside the option
	if o.Note != "" {
		b.WriteString("   ")
		b.WriteString(dim.Render(o.Note))
	}
	lines := []string{cut(strings.TrimRight(b.String(), " "), width)}
	// Say below the option what it is
	if o.Detail != "" {
		indent := "    "
		// Indent the detail under the label of a numbered or markable option
		if l.numbered {
			indent += "   "
		}
		// Indent the detail under the label of a markable option
		if l.marks != nil {
			indent += "    "
		}
		lines = append(lines, cut(indent+dim.Render(o.Detail), width))
	}
	return lines
}

// heading lines the header up with the rows below it, past the room their pointer, number and mark take.
func (l list) heading(label int, columns []int) string {
	var b strings.Builder
	b.WriteString("  ")
	// Leave the room a number takes
	if l.numbered {
		b.WriteString("   ")
	}
	// Leave the room a mark takes
	if l.marks != nil {
		b.WriteString("    ")
	}
	b.WriteString(pad(l.header[0], label))
	// Line every column's label up with its column
	for c, h := range l.header[1:] {
		b.WriteString("   ")
		// Pad a label to a column the rows hold
		if c < len(columns) {
			h = pad(h, columns[c])
		}
		b.WriteString(h)
	}
	return strings.TrimRight(b.String(), " ")
}

// widths measures the widest label and the widest value of each column across options and the header's labels.
func widths(options []menu.Option, header []string) (int, []int) {
	label := 0
	var columns []int
	// Count the header's labels in
	if len(header) > 0 {
		label = ansi.StringWidth(header[0])
		// Measure every column's label
		for _, h := range header[1:] {
			columns = append(columns, ansi.StringWidth(h))
		}
	}
	// Measure every option
	for _, o := range options {
		label = max(label, ansi.StringWidth(o.Shown()))
		// Measure every column of the option
		for c, col := range o.Columns {
			// Grow the columns to the number this option has
			for len(columns) <= c {
				columns = append(columns, 0)
			}
			columns[c] = max(columns[c], ansi.StringWidth(col))
		}
	}
	return label, columns
}

// pad fills a value with spaces to a width.
func pad(v string, width int) string {
	return v + strings.Repeat(" ", max(width-ansi.StringWidth(v), 0))
}

// cut shortens a line to a width, ending it in an ellipsis where it runs longer.
func cut(line string, width int) string {
	// Keep a line whole where the screen leaves no width yet or it fits
	if width <= 0 || ansi.StringWidth(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "…")
}

// containsFold reports whether a value contains a filter, ignoring case.
func containsFold(value, filter string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(filter))
}
