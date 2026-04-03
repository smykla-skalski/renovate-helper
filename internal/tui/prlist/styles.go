package prlist

import "charm.land/lipgloss/v2"

var (
	styleReady    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleConflict = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleFailed   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	stylePending  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("15"))
	styleBox      = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("237"))
)
