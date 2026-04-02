package detail

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

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
	width  int
	height int
}

func New(pr github.PR) Model {
	return Model{pr: pr}
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Key().Code {
		case tea.KeyDown:
			m.scroll++
		case tea.KeyUp:
			if m.scroll > 0 {
				m.scroll--
			}
		default:
			switch msg.String() {
			case "j":
				m.scroll++
			case "k":
				if m.scroll > 0 {
					m.scroll--
				}
			}
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scroll = max(0, m.scroll-3)
		case tea.MouseWheelDown:
			m.scroll += 3
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

	content := b.String()
	lines := strings.Split(content, "\n")

	if m.scroll >= len(lines) {
		m.scroll = max(0, len(lines)-1)
	}
	lines = lines[m.scroll:]

	visibleH := m.height - 4 // border top/bottom + padding top/bottom
	if visibleH > 0 && len(lines) > visibleH {
		lines = lines[:visibleH]
	}

	return styleBox.Width(m.width - 2).Height(m.height - 2).Render(strings.Join(lines, "\n"))
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
