package tui

import "charm.land/lipgloss/v2"

// Colors and styles shared across the onboarding and VM-list screens.
//
// The palette uses ANSI 256 colors so it renders acceptably on any terminal
// without depending on true-color support. Everything is tuned for dark
// backgrounds, the most common terminal profile.

var (
	// Colors
	colorTitle    = lipgloss.Color("99")  // purple
	colorMuted    = lipgloss.Color("243") // light gray
	colorAccent   = lipgloss.Color("213") // pink
	colorError    = lipgloss.Color("203") // red
	colorSuccess  = lipgloss.Color("42")  // green
	colorBorder   = lipgloss.Color("240") // dark gray
	colorSelected = lipgloss.Color("213") // pink
	colorIP       = lipgloss.Color("117") // light blue
)

// Pre-built styles used across screens. Style methods in lipgloss v2 return
// new values (the type is a value, not a pointer), so these are safe to use
// as read-only defaults and further customized per call.
var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorTitle)
	styleLabel    = lipgloss.NewStyle().Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleDesc     = lipgloss.NewStyle().Foreground(colorMuted)
	styleError    = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	styleHint     = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	styleSelected = lipgloss.NewStyle().Foreground(colorSelected).Bold(true)
	styleRunning  = lipgloss.NewStyle().Foreground(colorSuccess)
	styleStopped  = lipgloss.NewStyle().Foreground(colorMuted)
	styleIP       = lipgloss.NewStyle().Foreground(colorIP)
	styleFrame    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 2)

	styleInputBorder      = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colorBorder)
	styleInputBorderFocus = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colorSelected)
)
