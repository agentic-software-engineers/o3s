package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// nonBreakingHyphen holds a hyphenated word together while its line breaks, and nonBreakingSpace two words.
const (
	nonBreakingHyphen = "\u2011"
	nonBreakingSpace  = "\u00a0"
)

// pane holds the output of a dialog's commands, broken at words to its width and scrolled to its end as it grows.
type pane struct {
	lines []string
	width int
	view  viewport.Model
}

// add appends a line and keeps the end in view.
func (p *pane) add(line string) {
	p.lines = append(p.lines, colored(line))
	p.fill()
}

// redraw puts a line in the place of the last one, as a progress bar redraws itself.
func (p *pane) redraw(line string) {
	// Add the line where the pane is still empty
	if len(p.lines) == 0 {
		p.add(line)
		return
	}
	p.lines[len(p.lines)-1] = colored(line)
	p.fill()
}

// size fits the pane into the room the screen leaves it, breaking every line anew for its width.
func (p *pane) size(width, height int) {
	p.width = width
	p.view.SetWidth(width)
	p.view.SetHeight(max(height, minPaneLines))
	p.fill()
}

// fill shows every line broken at its words to the pane's width, and keeps the end in view.
func (p *pane) fill() {
	shown := make([]string, 0, len(p.lines))
	// Fit every line to the pane's width
	for _, line := range p.lines {
		shown = append(shown, wrapped(line, p.width)...)
	}
	p.view.SetContentLines(shown)
	p.view.GotoBottom()
}

// scroll follows the keys that page through long output.
func (p *pane) scroll(msg tea.Msg) {
	p.view, _ = p.view.Update(msg)
}

// overflows reports whether the output runs longer than the room the pane has.
func (p pane) overflows() bool {
	return p.view.TotalLineCount() > p.view.Height()
}

// View draws the lines in reach.
func (p pane) View() string {
	return p.view.View()
}

// wrapped breaks a line at its words to a width, going on under a listed item, and keeps it whole while the width is
// still unset.
func wrapped(line string, width int) []string {
	hang := hangOf(ansi.Strip(line))
	room := width - hang
	// Keep a line whole where the screen lacks room
	if width <= 0 || room <= 0 {
		return []string{line}
	}
	// Hold every word together across its hyphens, so a command reads as written
	held := strings.ReplaceAll(line, "-", nonBreakingHyphen)
	broken := ansi.Wrap(held, room, "")
	// Break a word too long for any line where it stands, so its item starts on the first line
	if longestWord(ansi.Strip(line)) > room {
		broken = ansi.Hardwrap(held, room, true)
	}
	lines := strings.Split(strings.NewReplacer(nonBreakingHyphen, "-", nonBreakingSpace, " ").Replace(broken), "\n")
	// Set every further line under the item it belongs to
	for i := 1; i < len(lines); i++ {
		lines[i] = strings.Repeat(" ", hang) + strings.TrimLeft(lines[i], " ")
	}
	return lines
}

// hangOf measures how far a line's further lines sit in: under the text of a listed item, and under its indent otherwise.
func hangOf(line string) int {
	text := strings.TrimLeft(line, " ")
	hang := len(line) - len(text)
	// Sit under the text that follows an item's sign
	for _, sign := range []string{"+ ", "- ", "~ "} {
		// Take the sign the item starts with
		if strings.HasPrefix(text, sign) {
			return hang + len(sign)
		}
	}
	return hang
}

// longestWord measures the widest word of a line.
func longestWord(line string) int {
	widest := 0
	// Measure every word
	for word := range strings.FieldsSeq(line) {
		widest = max(widest, ansi.StringWidth(word))
	}
	return widest
}

// colored marks the lines that add, remove, change or warn with the theme's colors.
func colored(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	// Color a line by the sign it starts with
	switch {
	case strings.HasPrefix(trimmed, "warning:"), strings.HasPrefix(trimmed, "error:"):
		return warning.Render(line)
	case strings.HasPrefix(trimmed, "+ "):
		return added.Render(line)
	case strings.HasPrefix(trimmed, "- "):
		return removed.Render(line)
	}
	return line
}
