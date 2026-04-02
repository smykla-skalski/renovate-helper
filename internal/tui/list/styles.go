package list

import "github.com/charmbracelet/lipgloss"

var (
	styleReady     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleConflict  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleFailed    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	stylePending   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleHeader    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	styleSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleSelected  = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("15"))
	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
)
