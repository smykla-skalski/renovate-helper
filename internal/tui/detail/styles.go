package detail

import "charm.land/lipgloss/v2"

var (
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	styleKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleSection = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleReady   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleMerged  = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleLabel   = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("6"))
	styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("237")).
			Padding(1, 2)
)
