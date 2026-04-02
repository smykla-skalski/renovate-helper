package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorGreen  = lipgloss.Color("2")
	colorRed    = lipgloss.Color("1")
	colorYellow = lipgloss.Color("3")
	colorGray   = lipgloss.Color("8")
	colorWhite  = lipgloss.Color("15")

	styleReady    = lipgloss.NewStyle().Foreground(colorGreen)
	styleConflict = lipgloss.NewStyle().Foreground(colorRed)
	stylePending  = lipgloss.NewStyle().Foreground(colorYellow)
	styleFailed   = lipgloss.NewStyle().Foreground(colorRed)
	styleDim      = lipgloss.NewStyle().Foreground(colorGray)
	styleBold     = lipgloss.NewStyle().Bold(true)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite)

	styleSeparator = lipgloss.NewStyle().Foreground(colorGray)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorGray).
			PaddingTop(1)

	styleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Foreground(colorWhite)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))
)
