package prlist

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

type sortMode int

const (
	sortByStatus sortMode = iota
	sortByAge
	sortByRepo
)

const (
	statusFailure  = "FAILURE"
	statusSuccess  = "SUCCESS"
	statusPending  = "PENDING"
	statusApproved = "APPROVED"

	mergeConflicting    = "CONFLICTING"
	reviewChanges       = "CHANGES_REQUESTED"
	reviewRequired      = "REVIEW_REQUIRED"
	checkCompleted      = "COMPLETED"
	conclusionTimedOut  = "TIMED_OUT"
	conclusionCancelled = "CANCELLED"
	conclusionNeutral   = "NEUTRAL"
	conclusionSkipped   = "SKIPPED"
)

type Model struct {
	selected map[int]bool
	fixing   map[string]bool
	filter   string
	prs      []github.PR
	filtered []github.PR
	cursor   int
	offset   int
	width    int
	height   int
	sort     sortMode
}

func New() Model {
	return Model{selected: make(map[int]bool), fixing: make(map[string]bool)}
}

func (m Model) SetFixing(prKey string, active bool) Model {
	if active {
		m.fixing[prKey] = true
	} else {
		delete(m.fixing, prKey)
	}
	return m
}

func (m Model) IsFixing(prKey string) bool {
	return m.fixing[prKey]
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) AllPRs() []github.PR {
	return m.prs
}

func (m Model) SetPRs(prs []github.PR) Model {
	m.prs = prs
	m.filtered = applyFilter(prs, m.filter)
	m.sortFiltered()
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	if m.offset > m.cursor {
		m.offset = m.cursor
	}
	return m
}

func (m Model) SetFilter(f string) Model {
	m.filter = f
	m.filtered = applyFilter(m.prs, f)
	m.sortFiltered()
	m.cursor = 0
	m.offset = 0
	return m
}

func (m Model) sortFiltered() {
	sortPRs(m.filtered, m.sort)
	stableSortByRepo(m.filtered)
}

func stableSortByRepo(prs []github.PR) {
	for i := 1; i < len(prs); i++ {
		for j := i; j > 0 && prs[j].Repo < prs[j-1].Repo; j-- {
			prs[j], prs[j-1] = prs[j-1], prs[j]
		}
	}
}

func (m Model) Selected() (github.PR, bool) {
	if len(m.filtered) == 0 {
		return github.PR{}, false
	}
	return m.filtered[m.cursor], true
}

func (m Model) SelectedPRs() []github.PR {
	var prs []github.PR
	for i, sel := range m.selected {
		if sel && i < len(m.filtered) {
			prs = append(prs, m.filtered[i])
		}
	}
	return prs
}

// SelectedPRsInGroup returns selected PRs belonging to the same repo as the
// currently focused PR.
func (m Model) SelectedPRsInGroup() []github.PR {
	if len(m.filtered) == 0 {
		return nil
	}
	repo := m.filtered[m.cursor].Repo
	var prs []github.PR
	for i, sel := range m.selected {
		if sel && i < len(m.filtered) && m.filtered[i].Repo == repo {
			prs = append(prs, m.filtered[i])
		}
	}
	return prs
}

// PRsNeedingApprovalInGroup returns PRs in the current repo group that need
// review and don't have failing checks.
func (m Model) PRsNeedingApprovalInGroup() []github.PR {
	if len(m.filtered) == 0 {
		return nil
	}
	repo := m.filtered[m.cursor].Repo
	var prs []github.PR
	for i := range m.filtered {
		pr := m.filtered[i]
		if pr.Repo == repo && pr.ReviewStatus == reviewRequired && pr.CheckStatus != statusFailure {
			prs = append(prs, pr)
		}
	}
	return prs
}

// AutoApprovablePRs returns all filtered PRs that can be auto-approved:
// checks passed, review required, not conflicting.
func (m Model) AutoApprovablePRs() []github.PR {
	var prs []github.PR
	for i := range m.filtered {
		pr := m.filtered[i]
		if pr.CheckStatus == statusSuccess && pr.ReviewStatus == reviewRequired && pr.Mergeable != mergeConflicting {
			prs = append(prs, pr)
		}
	}
	return prs
}

// AutoMergeablePRs returns all filtered PRs that are already approved and
// ready to merge: approved, checks passed, not conflicting.
func (m Model) AutoMergeablePRs() []github.PR {
	var prs []github.PR
	for i := range m.filtered {
		pr := m.filtered[i]
		if pr.ReviewStatus == statusApproved && pr.CheckStatus == statusSuccess && pr.Mergeable != mergeConflicting {
			prs = append(prs, pr)
		}
	}
	return prs
}

// CurrentRepo returns the repo of the currently focused PR.
func (m Model) CurrentRepo() string {
	if len(m.filtered) == 0 {
		return ""
	}
	return m.filtered[m.cursor].Repo
}

func (m Model) ClearSelected() Model {
	m.selected = make(map[int]bool)
	return m
}

func (m Model) visibleRows() int {
	v := m.height - 4 // header + border top/bottom + header outside box
	if v < 1 {
		return 1
	}
	return v
}

const compactThreshold = 80

func (m Model) compact() bool {
	return m.width < compactThreshold
}

func (m Model) columns() (colTitle, colStatus, colChecks, colFixing int) {
	colStatus, colChecks, colFixing = 12, 10, 7
	if m.compact() {
		colStatus = 3
	}
	// 2 (sel) + 4 (separators) + 4 (age) + 4 (box border/padding).
	fixed := 2 + colStatus + colChecks + colFixing + 4 + 4 + 4
	colTitle = m.width - fixed
	if colTitle < 20 {
		colTitle = 20
	}
	return colTitle, colStatus, colChecks, colFixing
}

func (m Model) moveUp(n int) Model {
	if len(m.filtered) == 0 {
		return m
	}
	m.cursor = max(0, m.cursor-n)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	return m
}

func (m Model) lastVisibleIndex(offset int) int {
	visible := m.visibleRows()
	rowsUsed := 0
	last := offset
	var lastRepo string
	for i := offset; i < len(m.filtered) && rowsUsed < visible; i++ {
		if m.filtered[i].Repo != lastRepo {
			lastRepo = m.filtered[i].Repo
			if rowsUsed+1 >= visible {
				break
			}
			rowsUsed++
		}
		rowsUsed++
		last = i
	}
	return last
}

func (m Model) moveDown(n int) Model {
	if len(m.filtered) == 0 {
		return m
	}
	m.cursor = min(len(m.filtered)-1, m.cursor+n)
	for m.cursor > m.lastVisibleIndex(m.offset) {
		m.offset++
	}
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, upKey):
			m = m.moveUp(1)
		case key.Matches(msg, downKey):
			m = m.moveDown(1)
		case key.Matches(msg, selectKey):
			m.selected[m.cursor] = !m.selected[m.cursor]
		case key.Matches(msg, sortKey):
			m.sort = (m.sort + 1) % 3
			m.sortFiltered()
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m = m.moveUp(3)
		case tea.MouseWheelDown:
			m = m.moveDown(3)
		}
	}
	return m, nil
}

var (
	upKey     = key.NewBinding(key.WithKeys("k", "up"))
	downKey   = key.NewBinding(key.WithKeys("j", "down"))
	selectKey = key.NewBinding(key.WithKeys(" "))
	sortKey   = key.NewBinding(key.WithKeys("s"))
)

func (m Model) View() string {
	if len(m.filtered) == 0 {
		return styleDim.Render("no PRs")
	}

	colTitle, colStatus, colChecks, colFixing := m.columns()
	statusLabel := "Status"
	if m.compact() {
		statusLabel = "St"
	}
	header := styleHeader.Render(
		"  " +
			padRight("Title", colTitle) + " " +
			padRight(statusLabel, colStatus) + " " +
			padRight("Checks", colChecks) + " " +
			padRight("Fixing", colFixing) + " " +
			"Age",
	)

	visible := m.visibleRows()
	start := m.offset

	var rows []string
	var lastRepo string
	rowsUsed := 0
	for i := start; i < len(m.filtered) && rowsUsed < visible; i++ {
		if m.filtered[i].Repo != lastRepo {
			lastRepo = m.filtered[i].Repo
			if rowsUsed+1 >= visible {
				break
			}
			rows = append(rows, styleHeader.Render(lastRepo))
			rowsUsed++
		}
		rows = append(rows, m.renderRow(i))
		rowsUsed++
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	inner := styleBox.Width(m.width - 2).Height(m.height - 3).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, header, inner)
}

// padRight pads s to width based on visual (rendered) width.
func padRight(s string, width int) string {
	vw := lipgloss.Width(s)
	if vw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vw)
}

func (m Model) renderRow(i int) string {
	pr := m.filtered[i]
	sel := "  "
	if m.selected[i] {
		sel = "● "
	}

	colTitle, colStatus, colChecks, colFixing := m.columns()

	title := truncate(pr.Title, colTitle-2)
	status := prStatus(pr, m.compact())
	checks := prChecks(pr)
	prKey := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
	fixing := styleDim.Render("-")
	if m.fixing[prKey] {
		fixing = styleReady.Render("\u26a1")
	}
	age := prAge(pr.CreatedAt)

	row := padRight(sel, 2) +
		padRight(title, colTitle) + " " +
		padRight(status, colStatus) + " " +
		padRight(checks, colChecks) + " " +
		padRight(fixing, colFixing) + " " +
		age

	if i == m.cursor {
		return styleSelected.Render(row)
	}
	return row
}

func prStatus(pr github.PR, compact bool) string {
	switch {
	case pr.Mergeable == mergeConflicting:
		if compact {
			return styleConflict.Render("✗")
		}
		return styleConflict.Render("✗ Conflict")
	case pr.CheckStatus == statusFailure:
		if compact {
			return styleFailed.Render("✗")
		}
		return styleFailed.Render("✗ Checks")
	case pr.ReviewStatus == reviewChanges:
		if compact {
			return styleConflict.Render("✗")
		}
		return styleConflict.Render("✗ Changes")
	case pr.CheckStatus == statusPending:
		if compact {
			return stylePending.Render("◐")
		}
		return stylePending.Render("◐ Checks")
	case pr.ReviewStatus == reviewRequired:
		if compact {
			return stylePending.Render("◐")
		}
		return stylePending.Render("◐ Review")
	case pr.ReviewStatus == statusApproved && pr.CheckStatus == statusSuccess:
		if compact {
			return styleReady.Render("✓")
		}
		return styleReady.Render("✓ Ready")
	default:
		if compact {
			return styleDim.Render("~")
		}
		return styleDim.Render("~ Pending")
	}
}

func prChecks(pr github.PR) string {
	total := len(pr.Checks)
	if total == 0 {
		return styleDim.Render("-")
	}
	passed := 0
	for i := range pr.Checks {
		c := pr.Checks[i].Conclusion
		if c == statusSuccess || c == conclusionNeutral || c == conclusionSkipped {
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
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func applyFilter(prs []github.PR, f string) []github.PR {
	if f == "" {
		out := make([]github.PR, len(prs))
		copy(out, prs)
		return out
	}
	f = strings.ToLower(f)
	var out []github.PR
	for i := range prs {
		if strings.Contains(strings.ToLower(prs[i].Repo), f) ||
			strings.Contains(strings.ToLower(prs[i].Title), f) ||
			strings.Contains(strings.ToLower(prs[i].ReviewStatus), f) {
			out = append(out, prs[i])
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
	case sortByStatus:
		return statusWeight(a) < statusWeight(b)
	case sortByAge:
		return a.CreatedAt.After(b.CreatedAt)
	case sortByRepo:
		return a.Repo < b.Repo
	}
	return statusWeight(a) < statusWeight(b)
}

func statusWeight(pr github.PR) int {
	switch {
	case pr.ReviewStatus == statusApproved && pr.CheckStatus == statusSuccess:
		return 0
	case pr.CheckStatus == statusPending:
		return 1
	case pr.ReviewStatus == reviewRequired:
		return 2
	case pr.CheckStatus == statusFailure:
		return 3
	case pr.Mergeable == mergeConflicting:
		return 4
	default:
		return 5
	}
}
