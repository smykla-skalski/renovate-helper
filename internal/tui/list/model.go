package list

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

type sortMode int

const (
	sortByStatus sortMode = iota
	sortByAge
	sortByRepo
)

// Model is the PR list view.
type Model struct {
	prs      []github.PR
	filtered []github.PR
	cursor   int
	selected map[int]bool
	filter   string
	sort     sortMode
	grouped  bool
	width    int
	height   int
}

func New() Model {
	return Model{selected: make(map[int]bool)}
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) SetPRs(prs []github.PR) Model {
	m.prs = prs
	m.filtered = applyFilter(prs, m.filter)
	sortPRs(m.filtered, m.sort)
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	return m
}

func (m Model) SetFilter(f string) Model {
	m.filter = f
	m.filtered = applyFilter(m.prs, f)
	sortPRs(m.filtered, m.sort)
	m.cursor = 0
	return m
}

func (m Model) Selected() (github.PR, bool) {
	if len(m.filtered) == 0 {
		return github.PR{}, false
	}
	return m.filtered[m.cursor], true
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, upKey):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, downKey):
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case key.Matches(msg, selectKey):
			m.selected[m.cursor] = !m.selected[m.cursor]
		case key.Matches(msg, sortKey):
			m.sort = (m.sort + 1) % 3
			sortPRs(m.filtered, m.sort)
		case key.Matches(msg, groupKey):
			m.grouped = !m.grouped
		}
	}
	return m, nil
}

var (
	upKey     = key.NewBinding(key.WithKeys("k", "up"))
	downKey   = key.NewBinding(key.WithKeys("j", "down"))
	selectKey = key.NewBinding(key.WithKeys(" "))
	sortKey   = key.NewBinding(key.WithKeys("s"))
	groupKey  = key.NewBinding(key.WithKeys("g"))
)

func (m Model) View() string {
	if len(m.filtered) == 0 {
		return styleDim.Render("no PRs")
	}

	header := styleHeader.Render(
		fmt.Sprintf("%-30s %-45s %-12s %-10s %s",
			"Repo", "Title", "Status", "Checks", "Age"),
	)
	sep := styleSeparator.Render(strings.Repeat("─", m.width))

	visible := m.height - 3
	if visible < 1 {
		visible = 1
	}
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	end := start + visible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	var rows []string
	for i := start; i < end; i++ {
		rows = append(rows, m.renderRow(i))
	}

	title := styleTitle.Render("gh-renovate-tracker") +
		styleDim.Render(fmt.Sprintf("  %d PRs", len(m.filtered)))

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		sep,
		header,
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
}

func (m Model) renderRow(i int) string {
	pr := m.filtered[i]
	sel := "  "
	if m.selected[i] {
		sel = "● "
	}

	status := prStatus(pr)
	checks := prChecks(pr)
	age := prAge(pr.CreatedAt)

	repo := truncate(pr.Repo, 28)
	title := truncate(pr.Title, 43)

	row := fmt.Sprintf("%s%-30s %-45s %-12s %-10s %s",
		sel, repo, title, status, checks, age)

	if i == m.cursor {
		return styleSelected.Render(row)
	}
	return row
}

func prStatus(pr github.PR) string {
	switch {
	case pr.Mergeable == "CONFLICTING":
		return styleConflict.Render("✗ Conflict")
	case pr.CheckStatus == "FAILURE":
		return styleFailed.Render("✗ Checks")
	case pr.ReviewStatus == "CHANGES_REQUESTED":
		return styleConflict.Render("✗ Changes")
	case pr.CheckStatus == "PENDING":
		return stylePending.Render("◐ Checks")
	case pr.ReviewStatus == "REVIEW_REQUIRED":
		return stylePending.Render("◐ Review")
	case pr.ReviewStatus == "APPROVED" && pr.CheckStatus == "SUCCESS":
		return styleReady.Render("✓ Ready")
	default:
		return styleDim.Render("~ Pending")
	}
}

func prChecks(pr github.PR) string {
	total := len(pr.Checks)
	if total == 0 {
		return styleDim.Render("-")
	}
	passed := 0
	for _, c := range pr.Checks {
		if c.Conclusion == "SUCCESS" || c.Conclusion == "NEUTRAL" || c.Conclusion == "SKIPPED" {
			passed++
		}
	}
	s := fmt.Sprintf("%d/%d", passed, total)
	switch {
	case passed == total:
		return styleReady.Render(s)
	case passed < total:
		return styleFailed.Render(s)
	default:
		return stylePending.Render(s)
	}
}

func prAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func applyFilter(prs []github.PR, f string) []github.PR {
	if f == "" {
		out := make([]github.PR, len(prs))
		copy(out, prs)
		return out
	}
	f = strings.ToLower(f)
	var out []github.PR
	for _, pr := range prs {
		if strings.Contains(strings.ToLower(pr.Repo), f) ||
			strings.Contains(strings.ToLower(pr.Title), f) ||
			strings.Contains(strings.ToLower(pr.ReviewStatus), f) {
			out = append(out, pr)
		}
	}
	return out
}

func sortPRs(prs []github.PR, mode sortMode) {
	for i := 1; i < len(prs); i++ {
		for j := i; j > 0 && less(prs[j], prs[j-1], mode); j-- {
			prs[j], prs[j-1] = prs[j-1], prs[j]
		}
	}
}

func less(a, b github.PR, mode sortMode) bool {
	switch mode {
	case sortByAge:
		return a.CreatedAt.After(b.CreatedAt)
	case sortByRepo:
		return a.Repo < b.Repo
	default:
		return statusWeight(a) < statusWeight(b)
	}
}

func statusWeight(pr github.PR) int {
	switch {
	case pr.ReviewStatus == "APPROVED" && pr.CheckStatus == "SUCCESS":
		return 0
	case pr.CheckStatus == "PENDING":
		return 1
	case pr.ReviewStatus == "REVIEW_REQUIRED":
		return 2
	case pr.CheckStatus == "FAILURE":
		return 3
	case pr.Mergeable == "CONFLICTING":
		return 4
	default:
		return 5
	}
}
