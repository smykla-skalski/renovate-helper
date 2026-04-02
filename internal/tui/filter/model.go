package filter

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the filter text input overlay.
type Model struct {
	input textinput.Model
	done  bool
}

func New() Model {
	ti := textinput.New()
	ti.Placeholder = "filter by repo, title, status..."
	ti.Focus()
	return Model{input: ti}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "enter" {
		m.done = true
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	return "/ " + m.input.View()
}

func (m Model) Value() string { return m.input.Value() }
func (m Model) Done() bool    { return m.done }
