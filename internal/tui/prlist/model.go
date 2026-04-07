package prlist

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
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
	sortIconFirst(m.filtered)
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
		if pr.ReviewStatus == statusApproved && pr.CheckStatus == statusSuccess && pr.Mergeable != mergeConflicting && !pr.StabilityDays {
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
	v := m.height - 1 - 2 // header outside box + border top/bottom.
	if v < 1 {
		return 1
	}
	return v
}

const compactThreshold = 80

func (m Model) compact() bool {
	return m.width < compactThreshold
}

type cols struct {
	title  int
	status int
	checks int
	fixing int
	age    int
	icon   int // 0 when no PR in the list carries a left-side icon
}

const (
	colSel    = 2 // selection indicator "● " or "  "
	colBorder = 2 // box left + right border
	colSeps   = 4 // spaces between title|status|checks|fixing|age
	colPadR   = 1 // right padding after last column
	colIconW  = 2 // left-icon glyph + trailing space, e.g. "⚠ "
)

// columns computes column widths from the actual filtered PR data. Each
// column is sized to fit its widest value (or its header, whichever is
// larger). Title gets whatever space remains.
func (m Model) columns() cols {
	compact := m.compact()

	// Start with header widths as minimums.
	statusMin := lipgloss.Width("Status")
	if compact {
		statusMin = lipgloss.Width("St")
	}
	c := cols{
		status: statusMin,
		checks: 1,
		fixing: 1,
		age:    1,
	}

	hasIcon := false
	for i := range m.filtered {
		pr := m.filtered[i]

		if prIcon(pr) != "" {
			hasIcon = true
		}
		if sw := lipgloss.Width(prStatus(pr, compact)); sw > c.status {
			c.status = sw
		}
		checksStr := prChecks(pr)
		if pr.StabilityDays {
			checksStr += stylePending.Render("⏳")
		}
		if cw := lipgloss.Width(checksStr); cw > c.checks {
			c.checks = cw
		}
		if aw := lipgloss.Width(prAge(pr.CreatedAt)); aw > c.age {
			c.age = aw
		}
		prKey := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
		if m.fixing[prKey] {
			if fw := lipgloss.Width("\u26a1"); fw > c.fixing {
				c.fixing = fw
			}
		}
	}

	if hasIcon {
		c.icon = colIconW
	}

	fixed := colSel + c.icon + c.status + c.checks + c.fixing + c.age + colSeps + colPadR + colBorder
	c.title = m.width - fixed
	if c.title < 20 {
		c.title = 20
	}
	return c
}

// cell truncates s to w display columns (with ellipsis if needed) and pads
// to exactly w columns. ANSI-aware and wide-char-safe.
func cell(s string, w int) string {
	return lipgloss.NewStyle().Width(w).MaxWidth(w).Inline(true).
		Render(ansi.Truncate(s, w, "…"))
}

// highlightCell renders a cell with the selected row style. Inner ANSI codes
// are stripped so the highlight background covers the full cell width.
func highlightCell(s string, w int) string {
	return styleSelected.Width(w).MaxWidth(w).Inline(true).
		Render(ansi.Truncate(ansi.Strip(s), w, "…"))
}

// centerCell renders s horizontally centered within w columns. It uses the
// same lipgloss wrapping as cell() for consistent ANSI handling.
func centerCell(s string, w int) string {
	t := ansi.Truncate(s, w, "…")
	if sw := lipgloss.Width(t); sw < w {
		t = strings.Repeat(" ", (w-sw)/2) + t
	}
	return lipgloss.NewStyle().Width(w).MaxWidth(w).Inline(true).Render(t)
}

// rowAtY maps a screen Y coordinate to a filtered PR index, accounting for
// the header line, box top border, and repo group header rows.
func (m Model) rowAtY(y int) (int, bool) {
	// y=0 is the column header, y=1 is the box top border.
	contentY := y - 2
	if contentY < 0 {
		return 0, false
	}

	row := 0
	var lastRepo string
	for i := m.offset; i < len(m.filtered); i++ {
		if m.filtered[i].Repo != lastRepo {
			lastRepo = m.filtered[i].Repo
			if row == contentY {
				return 0, false // clicked a repo group header
			}
			row++
		}
		if row == contentY {
			return i, true
		}
		row++
	}
	return 0, false
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
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			if idx, ok := m.rowAtY(tea.Mouse(msg).Y); ok {
				m.cursor = idx
			}
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

	c := m.columns()
	statusLabel := "Status"
	if m.compact() {
		statusLabel = "St"
	}
	iconHeader := ""
	if c.icon > 0 {
		iconHeader = cell("", c.icon)
	}
	header := styleHeader.Render(
		" " + strings.Repeat(" ", colSel) +
			iconHeader +
			cell("Title", c.title) + " " +
			cell(statusLabel, c.status) + " " +
			centerCell("⊘", c.checks) + " " +
			centerCell("⚙", c.fixing) + " " +
			centerCell("⏱", c.age),
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
			rows = append(rows, styleHeader.Render(" "+lastRepo))
			rowsUsed++
		}
		rows = append(rows, m.renderRow(i))
		rowsUsed++
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	inner := styleBox.Width(m.width).Height(m.height - 1).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, header, inner)
}

func (m Model) renderRow(i int) string {
	pr := m.filtered[i]
	sel := "  "
	if m.selected[i] {
		sel = "● "
	}

	c := m.columns()
	status := prStatus(pr, m.compact())
	checks := prChecks(pr)
	if pr.StabilityDays {
		checks += stylePending.Render("⏳")
	}
	prKey := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
	fixing := styleDim.Render("-")
	if m.fixing[prKey] {
		fixing = styleReady.Render("\u26a1")
	}
	age := prAge(pr.CreatedAt)

	iconGlyph := strings.Repeat(" ", c.icon) // blank placeholder preserves alignment
	if c.icon > 0 {
		if g := prIcon(pr); g != "" {
			iconGlyph = g
		}
	}

	if i == m.cursor {
		sep := styleSelected.Render(" ")
		iconCol := ""
		if c.icon > 0 {
			iconCol = highlightCell(iconGlyph, c.icon)
		}
		return highlightCell(sel, colSel) +
			iconCol +
			highlightCell(pr.Title, c.title) + sep +
			highlightCell(status, c.status) + sep +
			highlightCell(checks, c.checks) + sep +
			highlightCell(fixing, c.fixing) + sep +
			highlightCell(age, c.age) + styleSelected.Render(" ")
	}

	iconCol := ""
	if c.icon > 0 {
		iconCol = cell(iconGlyph, c.icon)
	}
	return cell(sel, colSel) +
		iconCol +
		cell(pr.Title, c.title) + " " +
		cell(status, c.status) + " " +
		cell(checks, c.checks) + " " +
		cell(fixing, c.fixing) + " " +
		cell(age, c.age) + " "
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

// prIcon returns the styled glyph for the left-icon column, or "" if none.
// Add new icon conditions here; the column width is automatically reserved for
// all rows in the list whenever any PR in the list returns a non-empty glyph.
func prIcon(pr github.PR) string {
	if isSecurity(pr) {
		return styleSecurity.Render("🔒")
	}
	return ""
}

// isSecurity returns true if the PR has a label containing "security".
func isSecurity(pr github.PR) bool {
	for _, l := range pr.Labels {
		if strings.Contains(strings.ToLower(l), "security") {
			return true
		}
	}
	return false
}

// sortIconFirst does a stable sort that puts PRs with a left-side icon before
// plain PRs within each repo group. Must be called after stableSortByRepo.
func sortIconFirst(prs []github.PR) {
	for i := 1; i < len(prs); i++ {
		for j := i; j > 0 && prs[j].Repo == prs[j-1].Repo && prIcon(prs[j]) != "" && prIcon(prs[j-1]) == ""; j-- {
			prs[j], prs[j-1] = prs[j-1], prs[j]
		}
	}
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
