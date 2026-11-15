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
