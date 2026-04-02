package detail

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

const (
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
	// content area = Width(m.width-2) - border_LR(2) - padding_LR(4)
	innerW := m.width - 8
	innerW = max(innerW, 10)
	divider := styleDim.Render(strings.Repeat("─", innerW))

	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "%s\n", styleTitle.Render(clip(pr.Title, innerW)))
	fmt.Fprintf(&b, "%s\n", styleDim.Render(clip(pr.URL, innerW)))
	fmt.Fprintf(&b, "\n%s\n\n", divider)

	// Metadata
	keyW := 13 // "%-11s  " = 11 + 2
	meta := func(key, val string) {
		fmt.Fprintf(&b, "%s  %s\n", styleKey.Render(fmt.Sprintf("%-11s", key)), val)
	}
	meta("REPO", clip(pr.Repo, innerW-keyW))
	meta("STATE", coloredState(pr.State))
	meta("MERGEABLE", coloredMergeable(pr.Mergeable))
	meta("REVIEW", coloredReview(pr.ReviewStatus))
	meta("DIFF",
		styleReady.Render(fmt.Sprintf("+%d", pr.Additions))+"  "+
			styleFailed.Render(fmt.Sprintf("-%d", pr.Deletions)),
	)
	if len(pr.Labels) > 0 {
		pills := make([]string, len(pr.Labels))
		for i, l := range pr.Labels {
			pills[i] = styleLabel.Render(" " + l + " ")
		}
		meta("LABELS", strings.Join(pills, " "))
	}

	// Checks
	if len(pr.Checks) > 0 {
		var failed []github.CheckRun
		for i := range pr.Checks {
			if pr.Checks[i].Conclusion == conclusionFailure || pr.Checks[i].Conclusion == conclusionTimedOut {
				failed = append(failed, pr.Checks[i])
			}
		}
		fmt.Fprintf(&b, "\n%s\n\n", divider)
		if len(failed) == 0 {
			fmt.Fprintf(&b, "%s  %s\n", styleSection.Render("CHECKS"), styleReady.Render("✓ all passed"))
		} else {
			fmt.Fprintf(&b, "%s  %s\n", styleSection.Render("CHECKS"), styleFailed.Render(fmt.Sprintf("✗ %d failed", len(failed))))
			for _, c := range failed {
				fmt.Fprintf(&b, "  %s  %s\n", styleFailed.Render("✗"), clip(c.Name, innerW-5))
			}
		}
	}

	// Reviews
	if len(pr.Reviews) > 0 {
		fmt.Fprintf(&b, "\n%s\n\n", divider)
		fmt.Fprintf(&b, "%s\n", styleSection.Render("REVIEWS"))
		for _, r := range pr.Reviews {
			fmt.Fprintf(&b, "  %-20s  %s\n", clip(r.Author, 20), coloredReviewState(r.State))
		}
	}

	content := b.String()
	lines := strings.Split(content, "\n")

	if m.scroll >= len(lines) {
		m.scroll = max(0, len(lines)-1)
	}
	lines = lines[m.scroll:]

	visibleH := m.height - 4 // padding top/bottom (2) + border top/bottom (2)
	if visibleH > 0 && len(lines) > visibleH {
		lines = lines[:visibleH]
	}

	return styleBox.Width(m.width - 2).Height(m.height).Render(strings.Join(lines, "\n"))
}

// clip truncates s to at most w runes, appending "…" if truncated.
func clip(s string, w int) string {
	if w <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= w {
		return s
	}
	return string(runes[:w-1]) + "…"
}

func coloredState(s string) string {
	switch s {
	case "OPEN":
		return styleReady.Render("● " + s)
	case "MERGED":
		return styleMerged.Render("⎇ " + s)
	case "CLOSED":
		return styleDim.Render("✕ " + s)
	default:
		return styleDim.Render(s)
	}
}

func coloredMergeable(s string) string {
	switch s {
	case "MERGEABLE":
		return styleReady.Render("✓ " + s)
	case "CONFLICTING":
		return styleFailed.Render("✗ " + s)
	default:
		return styleWarning.Render("◌ " + s)
	}
}

func coloredReview(s string) string {
	switch s {
	case "APPROVED":
		return styleReady.Render("✓ " + s)
	case "CHANGES_REQUESTED":
		return styleFailed.Render("✗ " + s)
	case "REVIEW_REQUIRED":
		return styleWarning.Render("⚠ " + s)
	default:
		return styleDim.Render(s)
	}
}

func coloredReviewState(s string) string {
	switch s {
	case "APPROVED":
		return styleReady.Render("✓ APPROVED")
	case "CHANGES_REQUESTED":
		return styleFailed.Render("✗ CHANGES REQUESTED")
	case "COMMENTED":
		return styleWarning.Render("◎ COMMENTED")
	case "DISMISSED":
		return styleDim.Render("◌ DISMISSED")
	default:
		return styleDim.Render(s)
	}
}
