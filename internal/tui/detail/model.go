package detail

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

// Model shows full PR details.
type Model struct {
	pr     github.PR
	scroll int
}

func New(pr github.PR) Model {
	return Model{pr: pr}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.scroll++
		case "k", "up":
			if m.scroll > 0 {
				m.scroll--
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	pr := m.pr
	var b strings.Builder

	b.WriteString(styleBold.Render(pr.Title) + "\n")
	b.WriteString(styleDim.Render(pr.URL) + "\n\n")

	b.WriteString(fmt.Sprintf("Repo:      %s\n", pr.Repo))
	b.WriteString(fmt.Sprintf("State:     %s\n", pr.State))
	b.WriteString(fmt.Sprintf("Mergeable: %s\n", pr.Mergeable))
	b.WriteString(fmt.Sprintf("Review:    %s\n", pr.ReviewStatus))
	b.WriteString(fmt.Sprintf("+%d / -%d\n\n", pr.Additions, pr.Deletions))

	if len(pr.Labels) > 0 {
		b.WriteString("Labels: " + strings.Join(pr.Labels, ", ") + "\n\n")
	}

	if len(pr.Checks) > 0 {
		b.WriteString(styleBold.Render("Checks:") + "\n")
		for _, c := range pr.Checks {
			icon := checkIcon(c)
			b.WriteString(fmt.Sprintf("  %s %s\n", icon, c.Name))
		}
		b.WriteString("\n")
	}

	if len(pr.Reviews) > 0 {
		b.WriteString(styleBold.Render("Reviews:") + "\n")
		for _, r := range pr.Reviews {
			b.WriteString(fmt.Sprintf("  %s: %s\n", r.Author, r.State))
		}
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func checkIcon(c github.CheckRun) string {
	switch c.Conclusion {
	case "SUCCESS":
		return styleReady.Render("✓")
	case "FAILURE", "TIMED_OUT":
		return styleFailed.Render("✗")
	default:
		return stylePending.Render("◐")
	}
}
