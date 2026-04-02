package detail

import "charm.land/lipgloss/v2"

var (
	styleReady   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	stylePending = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleBox     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("237")).
			Padding(1, 2)
)
