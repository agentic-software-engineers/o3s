package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// nonBreakingHyphen holds a hyphenated word together while its line breaks.
const nonBreakingHyphen = "\u2011"

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
		// Keep a line whole where the pane has no width yet
		if p.width <= 0 {
			shown = append(shown, line)
			continue
		}
		// Hold every word together across its hyphens, so a command reads as written
		held := strings.ReplaceAll(line, "-", nonBreakingHyphen)
		shown = append(shown, strings.Split(strings.ReplaceAll(ansi.Wrap(held, p.width, ""), nonBreakingHyphen, "-"), "\n")...)
	}
	p.view.SetContentLines(shown)
	p.view.GotoBottom()
}

// scroll follows the keys that page through long output.
func (p *pane) scroll(msg tea.Msg) {
	p.view, _ = p.view.Update(msg)
}

// View draws the lines in reach.
func (p pane) View() string {
	return p.view.View()
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
