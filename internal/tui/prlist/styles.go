package prlist

import "charm.land/lipgloss/v2"

var (
	styleReady    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleConflict = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleFailed   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	stylePending  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleSecurity = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("15"))
	styleBox      = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("237"))
	// styleStale is applied to every cell in a row whose cache is stale.
	// 238 is a very dark gray, clearly indicating the data is old/loading.
	styleStale = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	// styleStaleHeader is the same dim color applied to repo group headers.
	styleStaleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("238"))
)
