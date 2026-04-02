package help

import "github.com/charmbracelet/lipgloss"

// Model is the help overlay.
type Model struct{}

func New() Model { return Model{} }

func (m Model) View() string {
	content := `
  gh-renovate-tracker keybindings

  Navigation
    j/k / ↑↓   navigate
    enter       PR detail
    esc         back

  Actions
    m           merge selected PR
    M           merge all selected
    a           approve selected PR
    A           approve all selected
    r           rerun failed checks
    l           add label
    o           open in browser

  View
    /           filter
    s           cycle sort mode
    g           group by repo
    R           force refresh
    ?           this help
    q           quit
`
	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}
