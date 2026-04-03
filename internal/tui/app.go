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
	"github.com/klaudiush/gh-renovate-tracker/internal/tui/prlist"
)

type view int

const (
	viewList view = iota
	viewDetail
	viewFilter
	viewHelp
	viewLabel
	viewError
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
	statusUntil   time.Time
	help          help.Model
	client        *github.Client
	cfg           *config.Config
	pendingCmd    tea.Cmd
	status        string
	labelInput    textinput.Model
	spinner       spinner.Model
	filter        filter.Model
	labelPR       github.PR
	list          prlist.Model
	detail        detail.Model
	lastFetch     int64
	width         int
	height        int
	current       view
	loading       bool
	statusErr     bool
	confirming    bool
	confirmDanger bool
}

func New(client *github.Client, cfg *config.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		client:  client,
		cfg:     cfg,
		current: viewList,
		list:    prlist.New(),
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
		case viewFilter, viewHelp, viewLabel, viewError:
			// no scroll in these views
		}

	case prsLoadedMsg:
		m.loading = false
		m.lastFetch = time.Now().UnixNano()
		m.list = m.list.SetPRs(msg.prs)
		if time.Now().After(m.statusUntil) {
			m.status = fmt.Sprintf("%d PRs", len(msg.prs))
		}
		return m, tea.Every(m.cfg.RefreshInterval, func(t time.Time) tea.Msg {
			return fetchPRsCmd(m.client, m.cfg)()
		})

	case batchProgressMsg:
		m.status = fmt.Sprintf("%s %d/%d: %s", msg.verb, msg.done, msg.total, msg.cur)
		m.statusErr = false
		if msg.done < msg.total {
			return m, listenProgress(msg.ch)
		}
		return m, nil

	case fixCIReadyMsg:
		m.status = "launching claude..."
		m.list = m.list.SetFixing(msg.prKey, true)
		return m, fixCIExecCmd(msg.worktreeDir, msg.prompt, msg.prKey)

	case fixCIDoneMsg:
		m.list = m.list.SetFixing(msg.prKey, false)
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusErr = true
			return m, nil
		}
		m.status = "fix-ci done: " + msg.dir
		m.statusErr = false
		if repo, _, ok := strings.Cut(msg.prKey, "#"); ok {
			return m, fetchRepoPRsCmd(m.client, m.cfg, repo)
		}
		return m, fetchPRsCmd(m.client, m.cfg)

	case clipboardDoneMsg:
		m.status = fmt.Sprintf("copied %d links", msg.count)
		m.statusErr = false
		m.statusUntil = time.Now().Add(3 * time.Second)
		return m, nil

	case repoPRsLoadedMsg:
		m.loading = false
		existing := m.list.AllPRs()
		merged := make([]github.PR, 0, len(existing))
		for i := range existing {
			if existing[i].Repo != msg.repo {
				merged = append(merged, existing[i])
			}
		}
		merged = append(merged, msg.prs...)
		m.list = m.list.SetPRs(merged)
		if time.Now().After(m.statusUntil) {
			m.status = fmt.Sprintf("%d PRs", len(merged))
		}
		return m, nil

	case autoModeDoneMsg:
		m.status = fmt.Sprintf("auto: approved %d, merged %d PRs", msg.approved, msg.merged)
		m.statusErr = false
		m.statusUntil = time.Now().Add(5 * time.Second)
		if len(msg.repos) > 3 {
			return m, fetchPRsCmd(m.client, m.cfg)
		}
		cmds := make([]tea.Cmd, 0, len(msg.repos))
		for _, repo := range msg.repos {
			cmds = append(cmds, fetchRepoPRsCmd(m.client, m.cfg, repo))
		}
		return m, tea.Batch(cmds...)

	case actionDoneMsg:
		m.status = msg.msg
		m.statusErr = false
		if msg.repo == "" {
			return m, fetchPRsCmd(m.client, m.cfg)
		}
		return m, fetchRepoPRsCmd(m.client, m.cfg, msg.repo)

	case errMsg:
		m.loading = false
		m.status = msg.err.Error()
		m.statusErr = true
		if len(m.status) > 80 {
			m.current = viewError
		}
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
		if key.Matches(msg, keys.Quit) {
			return m, tea.Quit
		}
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

	case viewError:
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
		prs := m.list.SelectedPRsInGroup()
		if len(prs) == 0 {
			return m, nil
		}
		return m.startConfirm(
			fmt.Sprintf("Merge %d PRs in %s? (y/n)", len(prs), m.list.CurrentRepo()),
			batchMergeCmd(m.client, prs, m.cfg.MergeMethod),
		), nil

	case key.Matches(msg, keys.Approve):
		if pr, ok := m.list.Selected(); ok {
			return m, approvePRCmd(m.client, pr)
		}
		return m, nil

	case key.Matches(msg, keys.ApproveAll):
		prs := m.list.SelectedPRsInGroup()
		if len(prs) == 0 {
			return m, nil
		}
		return m, batchApproveCmd(m.client, prs)

	case key.Matches(msg, keys.Rerun):
		if pr, ok := m.list.Selected(); ok {
			return m, rerunChecksCmd(m.client, pr)
		}
		return m, nil

	case key.Matches(msg, keys.CopyLinks):
		prs := m.list.PRsNeedingApprovalInGroup()
		if len(prs) == 0 {
			m.status = "no PRs need approval"
			return m, nil
		}
		urls := make([]string, len(prs))
		for i := range prs {
			urls[i] = prs[i].URL
		}
		return m, copyToClipboardCmd(strings.Join(urls, "\n"), len(prs))

	case key.Matches(msg, keys.FixCI):
		if pr, ok := m.list.Selected(); ok {
			if pr.CheckStatus != "FAILURE" {
				m.status = "no failing checks"
				return m, nil
			}
			return m.startConfirm(
				fmt.Sprintf("Fix CI for %s#%d? (y/n)", pr.Repo, pr.Number),
				prepareFixCICmd(pr),
			), nil
		}
		return m, nil

	case key.Matches(msg, keys.Auto):
		toApprove := m.list.AutoApprovablePRs()
		toMerge := m.list.AutoMergeablePRs()
		total := len(toApprove) + len(toMerge)
		if total == 0 {
			m.status = "no PRs eligible for auto mode"
			return m, nil
		}
		return m.startDangerConfirm(
			fmt.Sprintf("Will approve %d and merge %d PRs without review. Proceed? (y/n)",
				len(toApprove), len(toApprove)+len(toMerge)),
			autoModeCmd(m.client, toApprove, toMerge, m.cfg.MergeMethod),
		), nil

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

func (m Model) startDangerConfirm(msg string, cmd tea.Cmd) Model {
	m = m.startConfirm(msg, cmd)
	m.confirmDanger = true
	return m
}

func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		cmd := m.pendingCmd
		m.confirming = false
		m.confirmDanger = false
		m.pendingCmd = nil
		m.status = ""
		return m, cmd
	default:
		m.confirming = false
		m.confirmDanger = false
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
	case viewList, viewError:
		body = m.list.View()
	}

	bottom := m.renderBottomBar()
	content := lipgloss.JoinVertical(lipgloss.Left, body, bottom)

	switch {
	case m.confirming:
		content = m.renderPopup()
	case m.current == viewError:
		content = m.renderErrorPopup()
	case m.loading && m.lastFetch == 0:
		content = m.renderLoadingPopup()
	}

	v := tea.NewView(content)
	v.WindowTitle = fmt.Sprintf("gh-renovate-tracker — %s", m.status)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m Model) renderLoadingPopup() string {
	msg := m.spinner.View() + " Fetching PRs…"

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("4")).
		Padding(1, 3).
		Render(msg)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "))
}

func (m Model) renderPopup() string {
	color := lipgloss.Color("3")
	titleText := "Confirm"
	if m.confirmDanger {
		color = lipgloss.Color("1")
		titleText = "⚠ WARNING"
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(color).Render(titleText)
	msg := m.status
	hint := styleDim.Render("y to confirm · any key to cancel")

	inner := lipgloss.JoinVertical(lipgloss.Center, title, "", msg, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(1, 3).
		Render(inner)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "))
}

func (m Model) renderErrorPopup() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Render("Error")
	hint := styleDim.Render("any key to dismiss")

	boxW := min(m.width-4, 100)
	innerW := boxW - 8 // padding + border.
	if innerW < 20 {
		innerW = 20
	}

	wrapped := lipgloss.NewStyle().Width(innerW).Render(m.status)
	inner := lipgloss.JoinVertical(lipgloss.Center, title, "", wrapped, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("1")).
		Padding(1, 3).
		Render(inner)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "))
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
			helpHint("m/M", "merge"),
			helpHint("a/A", "approve"),
			helpHint("c", "copy links"),
			helpHint("f", "fix CI"),
			helpHint("/", "filter"),
			helpHint("o", "open"),
			helpHint("!", "auto"),
			helpHint("?", "help"),
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
	case viewHelp, viewError:
		hints = []string{
			helpHint("any key", "close"),
		}
	}

	helpLine := strings.Join(hints, sep)

	// Truncate hints if they'd push status off screen
	maxHelp := m.width - 30 // reserve space for status
	if maxHelp > 0 && lipgloss.Width(helpLine) > maxHelp {
		helpLine = helpLine[:maxHelp] + styleDim.Render("…")
	}

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

	pad := 2
	innerW := m.width - 2*pad
	gap := max(2, innerW-lipgloss.Width(helpLine)-lipgloss.Width(status))
	lr := strings.Repeat(" ", pad)

	return styleBottomBar.Render(lr + helpLine + strings.Repeat(" ", gap) + status + lr)
}
