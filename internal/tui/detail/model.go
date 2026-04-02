package detail

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

const (
	conclusionSuccess  = "SUCCESS"
	conclusionFailure  = "FAILURE"
	conclusionTimedOut = "TIMED_OUT"
)

type Model struct {
	pr     github.PR
	scroll int
}

func New(pr github.PR) Model {
	return Model{pr: pr}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
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

	fmt.Fprintf(&b, "%s\n", styleBold.Render(pr.Title))
	fmt.Fprintf(&b, "%s\n\n", styleDim.Render(pr.URL))

	fmt.Fprintf(&b, "Repo:      %s\n", pr.Repo)
	fmt.Fprintf(&b, "State:     %s\n", pr.State)
	fmt.Fprintf(&b, "Mergeable: %s\n", pr.Mergeable)
	fmt.Fprintf(&b, "Review:    %s\n", pr.ReviewStatus)
	fmt.Fprintf(&b, "+%d / -%d\n\n", pr.Additions, pr.Deletions)

	if len(pr.Labels) > 0 {
		fmt.Fprintf(&b, "Labels: %s\n\n", strings.Join(pr.Labels, ", "))
	}

	if len(pr.Checks) > 0 {
		fmt.Fprintf(&b, "%s\n", styleBold.Render("Checks:"))
		for i := range pr.Checks {
			icon := checkIcon(pr.Checks[i])
			fmt.Fprintf(&b, "  %s %s\n", icon, pr.Checks[i].Name)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(pr.Reviews) > 0 {
		fmt.Fprintf(&b, "%s\n", styleBold.Render("Reviews:"))
		for i := range pr.Reviews {
			fmt.Fprintf(&b, "  %s: %s\n", pr.Reviews[i].Author, pr.Reviews[i].State)
		}
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func checkIcon(c github.CheckRun) string {
	switch c.Conclusion {
	case conclusionSuccess:
		return styleReady.Render("✓")
	case conclusionFailure, conclusionTimedOut:
		return styleFailed.Render("✗")
	default:
		return stylePending.Render("◐")
	}
}
