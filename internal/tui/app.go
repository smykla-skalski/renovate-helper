package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/klaudiush/gh-renovate-tracker/internal/config"
	"github.com/klaudiush/gh-renovate-tracker/internal/github"
	"github.com/klaudiush/gh-renovate-tracker/internal/tui/detail"
	"github.com/klaudiush/gh-renovate-tracker/internal/tui/filter"
	"github.com/klaudiush/gh-renovate-tracker/internal/tui/help"
	"github.com/klaudiush/gh-renovate-tracker/internal/tui/list"
)

type view int

const (
	viewList view = iota
	viewDetail
	viewFilter
	viewHelp
	viewLabel
)

var (
	styleFailed = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleReady  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	styleBottomBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
	styleHelpKey = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Bold(true)
	styleHelpDesc = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
	styleHelpSep = lipgloss.NewStyle().
			Foreground(lipgloss.Color("237"))
)

type Model struct {
	help       help.Model
	cfg        *config.Config
	pendingCmd tea.Cmd
	client     *github.Client
	status     string
	labelInput textinput.Model
	spinner    spinner.Model
	filter     filter.Model
	labelPR    github.PR
	detail     detail.Model
	list       list.Model
	lastFetch  int64
	width      int
	height     int
	current    view
	loading    bool
	statusErr  bool
	confirming bool
}

func New(client *github.Client, cfg *config.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		client:  client,
		cfg:     cfg,
		current: viewList,
		list:    list.New(),
		filter:  filter.New(),
		help:    help.New(),
		spinner: s,
		loading: true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchPRsCmd(m.client, m.cfg),
		m.spinner.Tick,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentH := msg.Height - 1 // 1 line for bottom bar
		m.list = m.list.SetSize(msg.Width, contentH)
		m.detail = m.detail.SetSize(msg.Width, contentH)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseWheelMsg:
		switch m.current {
		case viewList:
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		case viewDetail:
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		case viewFilter, viewHelp, viewLabel:
			// no scroll in these views
		}

	case prsLoadedMsg:
		m.loading = false
		m.lastFetch = time.Now().UnixNano()
		m.list = m.list.SetPRs(msg.prs)
		m.status = fmt.Sprintf("%d PRs", len(msg.prs))
		return m, tea.Every(m.cfg.RefreshInterval, func(t time.Time) tea.Msg {
			return fetchPRsCmd(m.client, m.cfg)()
		})

	case actionDoneMsg:
		m.status = msg.msg
		m.statusErr = false
		return m, fetchPRsCmd(m.client, m.cfg)

	case errMsg:
		m.loading = false
		m.status = msg.err.Error()
		m.statusErr = true
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Confirmation prompt intercepts all keys.
	if m.confirming {
		return m.handleConfirm(msg)
	}

	switch m.current {
	case viewDetail:
		if key.Matches(msg, keys.Esc) {
			m.current = viewList
			return m, nil
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd

	case viewFilter:
		if key.Matches(msg, keys.Esc) {
			m.list = m.list.SetFilter("")
			m.current = viewList
			return m, nil
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Done() {
			m.list = m.list.SetFilter(m.filter.Value())
			m.current = viewList
		}
		return m, cmd

	case viewHelp:
		m.current = viewList
		return m, nil

	case viewLabel:
		return m.handleLabelInput(msg)

	case viewList:
		// handled below
	}

	// viewList.
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Esc):
		m.list = m.list.SetFilter("")
		return m, nil

	case key.Matches(msg, keys.Help):
		m.current = viewHelp
		return m, nil

	case key.Matches(msg, keys.Filter):
		m.current = viewFilter
		m.filter = filter.New()
		return m, nil

	case key.Matches(msg, keys.Refresh):
		m.loading = true
		return m, fetchPRsCmd(m.client, m.cfg)

	case key.Matches(msg, keys.Enter):
		if pr, ok := m.list.Selected(); ok {
			m.detail = detail.New(pr).SetSize(m.width, m.height-1)
			m.current = viewDetail
		}
		return m, nil

	case key.Matches(msg, keys.Open):
		if pr, ok := m.list.Selected(); ok {
			return m, openBrowserCmd(pr.URL)
		}
		return m, nil

	case key.Matches(msg, keys.Merge):
		if pr, ok := m.list.Selected(); ok {
			return m.startConfirm(
				fmt.Sprintf("Merge %s#%d? (y/n)", pr.Repo, pr.Number),
				mergePRCmd(m.client, pr, m.cfg.MergeMethod),
			), nil
		}
		return m, nil

	case key.Matches(msg, keys.MergeAll):
		prs := m.list.SelectedPRs()
		if len(prs) == 0 {
			return m, nil
		}
		return m.startConfirm(
			fmt.Sprintf("Merge %d PRs? (y/n)", len(prs)),
			batchMergeCmd(m.client, prs, m.cfg.MergeMethod),
		), nil

	case key.Matches(msg, keys.Approve):
		if pr, ok := m.list.Selected(); ok {
			return m, approvePRCmd(m.client, pr)
		}
		return m, nil

	case key.Matches(msg, keys.ApproveAll):
		prs := m.list.SelectedPRs()
		if len(prs) == 0 {
			return m, nil
		}
		return m, batchApproveCmd(m.client, prs)

	case key.Matches(msg, keys.Rerun):
		if pr, ok := m.list.Selected(); ok {
			return m, rerunChecksCmd(m.client, pr)
		}
		return m, nil

	case key.Matches(msg, keys.Label):
		if pr, ok := m.list.Selected(); ok {
			ti := textinput.New()
			ti.Placeholder = "label name..."
			blinkCmd := ti.Focus()
			m.labelInput = ti
			m.labelPR = pr
			m.current = viewLabel
			return m, blinkCmd
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
}

func (m Model) startConfirm(msg string, cmd tea.Cmd) Model {
	m.confirming = true
	m.status = msg
	m.pendingCmd = cmd
	return m
}

func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		cmd := m.pendingCmd
		m.confirming = false
		m.pendingCmd = nil
		m.status = ""
		return m, cmd
	default:
		m.confirming = false
		m.pendingCmd = nil
		m.status = "cancelled"
		return m, nil
	}
}

func (m Model) handleLabelInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Esc) {
		m.current = viewList
		return m, nil
	}
	if msg.String() == "enter" {
		label := m.labelInput.Value()
		if label != "" {
			m.current = viewList
			return m, addLabelCmd(m.client, m.labelPR, label)
		}
		m.current = viewList
		return m, nil
	}
	var cmd tea.Cmd
	m.labelInput, cmd = m.labelInput.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View {
	var body string
	switch m.current {
	case viewDetail:
		body = m.detail.View()
	case viewFilter:
		body = m.list.View() + "\n" + m.filter.View()
	case viewHelp:
		body = m.help.View()
	case viewLabel:
		body = m.list.View() + "\n  label: " + m.labelInput.View()
	case viewList:
		body = m.list.View()
	}

	bottom := m.renderBottomBar()
	content := lipgloss.JoinVertical(lipgloss.Left, body, bottom)
	v := tea.NewView(content)
	v.WindowTitle = fmt.Sprintf("gh-renovate-tracker — %s", m.status)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func helpHint(k, desc string) string {
	return styleHelpKey.Render(k) + " " + styleHelpDesc.Render(desc)
}

func (m Model) renderBottomBar() string {
	sep := styleHelpSep.Render(" · ")
	var hints []string

	switch m.current {
	case viewList:
		hints = []string{
			helpHint("j/k", "navigate"),
			helpHint("space", "select"),
			helpHint("enter", "detail"),
			helpHint("m/M", "merge"),
			helpHint("a/A", "approve"),
			helpHint("/", "filter"),
			helpHint("s", "sort"),
			helpHint("g", "group"),
			helpHint("o", "open"),
			helpHint("esc", "clear filter"),
			helpHint("?", "help"),
			helpHint("q", "quit"),
		}
	case viewDetail:
		hints = []string{
			helpHint("j/k", "scroll"),
			helpHint("esc", "back"),
		}
	case viewFilter:
		hints = []string{
			helpHint("enter", "apply"),
			helpHint("esc", "cancel"),
		}
	case viewLabel:
		hints = []string{
			helpHint("enter", "confirm"),
			helpHint("esc", "cancel"),
		}
	case viewHelp:
		hints = []string{
			helpHint("any key", "close"),
		}
	}

	helpLine := strings.Join(hints, sep)

	var status string
	switch {
	case m.confirming:
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Render(m.status)
	case m.loading:
		status = m.spinner.View() + " loading…"
	case m.statusErr:
		status = styleFailed.Render("✗ " + m.status)
	case m.status != "":
		status = styleReady.Render("✓ " + m.status)
	default:
		ago := time.Since(time.Unix(0, m.lastFetch)).Round(time.Second)
		status = styleDim.Render(fmt.Sprintf("↻ %s ago", ago))
	}

	gap := max(2, m.width-lipgloss.Width(helpLine)-lipgloss.Width(status))

	return styleBottomBar.Render(helpLine + strings.Repeat(" ", gap) + status)
}
