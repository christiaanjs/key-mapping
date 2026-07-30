package main

import "github.com/charmbracelet/lipgloss"

// Palette. Kept to basic ANSI colors so the UI looks reasonable on both
// light and dark terminal themes without needing truecolor support.
var (
	styleDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))                            // green
	styleCurrent = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Underline(true) // yellow
	stylePending = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))                            // gray
	styleDanger  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))                            // red
	styleAccent  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)                 // cyan
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleBold    = lipgloss.NewStyle().Bold(true)

	styleTabActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Padding(0, 1)
	styleTabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1)

	styleChipActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Padding(0, 1)
	styleChipInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1)

	styleKeycap = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("7")).Padding(0, 1)

	styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(1, 2)

	styleStatLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleStatVal   = lipgloss.NewStyle().Bold(true)

	styleHelp = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// chip renders a small pill-like label, highlighted when active.
func chip(label string, active bool) string {
	if active {
		return styleChipActive.Render(label)
	}
	return styleChipInactive.Render(label)
}

// keycap renders a physical key as a small rectangular badge.
func keycap(label string) string {
	return styleKeycap.Render(label)
}
