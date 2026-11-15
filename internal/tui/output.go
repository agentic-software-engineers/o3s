package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// pane holds the output of a dialog's commands, scrolled to its end as it grows.
type pane struct {
	lines []string
	view  viewport.Model
}

// add appends a line and keeps the end in view.
func (p *pane) add(line string) {
	p.lines = append(p.lines, colored(line))
	p.view.SetContentLines(p.lines)
	p.view.GotoBottom()
}

// size fits the pane into the room the screen leaves it.
func (p *pane) size(width, height int) {
	p.view.SetWidth(width)
	p.view.SetHeight(max(height, 3))
	p.view.SetContentLines(p.lines)
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
