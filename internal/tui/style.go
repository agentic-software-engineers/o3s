// Package tui shows the menu's dialogs and runs their commands in one full-screen app.
package tui

import "charm.land/lipgloss/v2"

// The app's one theme.
var (
	accent    = lipgloss.Color("6")
	header    = lipgloss.NewStyle().Bold(true)
	dim       = lipgloss.NewStyle().Faint(true)
	title     = lipgloss.NewStyle().Bold(true)
	pointer   = lipgloss.NewStyle().Foreground(accent).Bold(true)
	marked    = lipgloss.NewStyle().Foreground(accent)
	warning   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	added     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	removed   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	separator = dim.Render(" / ")
)

// Sizes the screen holds to, whatever the terminal reports.
const (
	// chromeLines are the lines the header, the trail, the gaps and the hint take around the body.
	chromeLines = 5
	// minPaneLines keeps room to read output on the smallest screen.
	minPaneLines = 3
	// fieldMargin leaves room beside a text field, and minFieldWidth keeps it wide enough to type in.
	fieldMargin, minFieldWidth = 4, 10
)
