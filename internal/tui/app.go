package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
)

type Model struct {
	client    *github.Client
	cfg       *config.Config
	current   view
	list      list.Model
	detail    detail.Model
	filter    filter.Model
	help      help.Model
	spinner   spinner.Model
	loading   bool
	status    string
	statusErr bool
	lastFetch time.Time
	width     int
	height    int
}

// New creates the root TUI model.
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
		tea.SetWindowTitle("gh-renovate-tracker"),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list = m.list.SetSize(msg.Width, msg.Height-3)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case prsLoadedMsg:
		m.loading = false
		m.lastFetch = time.Now()
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
	}

	// viewList
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

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
			m.detail = detail.New(pr)
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
			return m, mergePRCmd(m.client, pr, m.cfg.MergeMethod)
		}
		return m, nil

	case key.Matches(msg, keys.Approve):
		if pr, ok := m.list.Selected(); ok {
			return m, approvePRCmd(m.client, pr)
		}
		return m, nil

	case key.Matches(msg, keys.Rerun):
		if pr, ok := m.list.Selected(); ok {
			return m, rerunChecksCmd(m.client, pr)
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
}

func (m Model) View() string {
	var body string
	switch m.current {
	case viewDetail:
		body = m.detail.View()
	case viewFilter:
		body = m.list.View() + "\n" + m.filter.View()
	case viewHelp:
		body = m.help.View()
	default:
		body = m.list.View()
	}

	status := m.renderStatus()
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

func (m Model) renderStatus() string {
	var s string
	if m.loading {
		s = m.spinner.View() + " loading..."
	} else if m.statusErr {
		s = styleFailed.Render("✗ " + m.status)
	} else if m.status != "" {
		s = styleReady.Render("✓ " + m.status)
	} else {
		ago := time.Since(m.lastFetch).Round(time.Second)
		s = styleDim.Render(fmt.Sprintf("↻ %s ago", ago))
	}
	return styleStatusBar.Render(s)
}
