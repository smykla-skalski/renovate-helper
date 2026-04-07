package tui

import (
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/smykla-skalski/gh-renovate-helper/internal/cache"
	"github.com/smykla-skalski/gh-renovate-helper/internal/config"
	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
	"github.com/smykla-skalski/gh-renovate-helper/internal/tui/detail"
	"github.com/smykla-skalski/gh-renovate-helper/internal/tui/filter"
	"github.com/smykla-skalski/gh-renovate-helper/internal/tui/help"
	"github.com/smykla-skalski/gh-renovate-helper/internal/tui/prlist"
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
	statusUntil    time.Time
	help           help.Model
	client         *github.Client
	cfg            *config.Config
	cache          *cache.Cache
	scheduledRepos map[string]bool
	fetchingRepos  map[string]bool // repos with an in-flight network request
	pendingCmd     tea.Cmd
	status         string
	labelInput     textinput.Model
	spinner        spinner.Model
	filter         filter.Model
	labelPR        github.PR
	list           prlist.Model
	detail         detail.Model
	lastFetch      int64
	width          int
	height         int
	current        view
	loading        bool
	statusErr      bool
	confirming     bool
	confirmDanger  bool
}

func New(client *github.Client, cfg *config.Config, c *cache.Cache) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	list := prlist.New().SetRepoOrder(cfg.Repos, cfg.Orgs)
	loading := true
	cachedPRs := c.AllPRs()
	if len(cachedPRs) > 0 {
		list = list.SetPRs(cachedPRs)
		list = list.SetStaleRepos(computeSpinningRepos(c, cfg, nil))
		loading = false
	}

	// Show repos from config that are not yet in cache as loading rows so the
	// user sees all expected repos from the start.
	initialLoading := make(map[string]bool)
	for _, r := range cfg.Repos {
		if _, ok := c.Get(r); !ok {
			initialLoading[r] = true
		}
	}
	if len(initialLoading) > 0 {
		list = list.SetLoadingRepos(initialLoading)
		loading = true
	}

	// Seed lastFetch from the most recent cache entry so the bottom bar shows
	// "↻ X ago" immediately on warm start instead of "↻ syncing…".
	var lastFetch int64
	for _, repo := range c.Repos() {
		if entry, ok := c.Get(repo); ok && entry.FetchedAt.UnixNano() > lastFetch {
			lastFetch = entry.FetchedAt.UnixNano()
		}
	}

	// Seed the spinner frame so the first render shows a glyph rather than a
	// blank icon (spinner.TickMsg fires after the initial paint).
	list = list.SetSpinnerFrame(s.View())

	return Model{
		client:         client,
		cfg:            cfg,
		cache:          c,
		current:        viewList,
		list:           list,
		filter:         filter.New(),
		help:           help.New(),
		spinner:        s,
		loading:        loading,
		lastFetch:      lastFetch,
		scheduledRepos: make(map[string]bool),
		fetchingRepos:  make(map[string]bool),
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick}

	// Collect all known repos (from cache + explicit cfg.Repos), deduplicated.
	knownRepos := m.cache.Repos()
	repoSet := make(map[string]bool, len(knownRepos)+len(m.cfg.Repos))
	for _, r := range knownRepos {
		repoSet[r] = true
	}
	for _, r := range m.cfg.Repos {
		repoSet[r] = true
	}
	repos := make([]string, 0, len(repoSet))
	for r := range repoSet {
		repos = append(repos, r)
	}

	n := len(repos)
	for i, repo := range repos {
		cmds = append(cmds, scheduledRepoRefreshCmd(repo, initialRepoDelay(m.cache, repo, m.cfg, i, n)))
		m.scheduledRepos[repo] = true
	}

	// Org discovery for finding new repos not yet in cache.
	for i, org := range m.cfg.Orgs {
		jitter := randomJitter(i, len(m.cfg.Orgs), m.cfg.RefreshInterval)
		cmds = append(cmds, scheduledOrgDiscoverCmd(m.client, m.cfg, org, jitter))
	}

	return tea.Batch(cmds...)
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

	case tea.MouseClickMsg:
		if m.current == viewList {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

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

	case repoFetchStartedMsg:
		m.fetchingRepos[msg.repo] = true
		m.list = m.list.SetStaleRepos(computeSpinningRepos(m.cache, m.cfg, m.fetchingRepos))
		return m, fetchRepoPRsCmdWith(m.client, m.cfg, msg.repo)

	case orgDiscoveredMsg:
		now := msg.fetchedAt
		var cmds []tea.Cmd
		for repo, prs := range msg.reposPRs {
			m.cache.Set(repo, prs, now)
			if !m.scheduledRepos[repo] {
				m.scheduledRepos[repo] = true
				jitter := safeJitter(m.cfg.RefreshInterval)
				cmds = append(cmds, scheduledRepoRefreshCmd(repo, m.cfg.RefreshInterval+jitter))
			}
		}
		m.list = m.list.SetPRs(m.cache.AllPRs())
		m.list = m.list.SetLoadingRepos(computeLoadingRepos(m.cache, m.scheduledRepos))
		m.list = m.list.SetStaleRepos(computeSpinningRepos(m.cache, m.cfg, m.fetchingRepos))
		// Always update lastFetch so the bottom bar shows "↻ X ago" and not
		// "↻ syncing…" when org discovery is the first data to arrive on cold start.
		m.lastFetch = now.UnixNano()
		if err := m.cache.Save(); err != nil {
			slog.Error("cache save failed", "error", err)
		}
		jitter := safeJitter(m.cfg.RefreshInterval / 4)
		cmds = append(cmds, scheduledOrgDiscoverCmd(m.client, m.cfg, msg.org, m.cfg.RefreshInterval+jitter))
		return m, tea.Batch(cmds...)

	case cacheClearedMsg:
		if err := m.cache.Clear(); err != nil {
			slog.Error("cache clear failed", "error", err)
		}
		m.list = m.list.SetPRs([]github.PR{})
		m.list = m.list.SetStaleRepos(nil)
		m.loading = true
		m.lastFetch = 0
		m.scheduledRepos = make(map[string]bool)
		m.fetchingRepos = make(map[string]bool)
		var cmds []tea.Cmd
		for i, repo := range m.cfg.Repos {
			delay := time.Duration(i) * 200 * time.Millisecond
			cmds = append(cmds, scheduledRepoRefreshCmd(repo, delay))
			m.scheduledRepos[repo] = true
		}
		for i, org := range m.cfg.Orgs {
			delay := time.Duration(i) * 300 * time.Millisecond
			cmds = append(cmds, scheduledOrgDiscoverCmd(m.client, m.cfg, org, delay))
		}
		// All scheduled repos have no cache data - show them as loading rows.
		m.list = m.list.SetLoadingRepos(computeLoadingRepos(m.cache, m.scheduledRepos))
		return m, tea.Batch(cmds...)

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
		delete(m.fetchingRepos, msg.repo)
		m.cache.Set(msg.repo, msg.prs, msg.fetchedAt)
		m.list = m.list.SetPRs(m.cache.AllPRs())
		m.list = m.list.SetLoadingRepos(computeLoadingRepos(m.cache, m.scheduledRepos))
		m.list = m.list.SetStaleRepos(computeSpinningRepos(m.cache, m.cfg, m.fetchingRepos))
		m.lastFetch = msg.fetchedAt.UnixNano()
		if m.loading {
			m.loading = false
		}
		if time.Now().After(m.statusUntil) {
			m.status = fmt.Sprintf("%d PRs", len(m.cache.AllPRs()))
		}
		if err := m.cache.Save(); err != nil {
			slog.Error("cache save failed", "error", err)
		}
		jitter := safeJitter(m.cfg.RefreshInterval / 5)
		return m, scheduledRepoRefreshCmd(msg.repo, m.cfg.RefreshInterval+jitter)

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
		m.list = m.list.SetSpinnerFrame(m.spinner.View())
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
			_ = m.cache.Save()
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
		_ = m.cache.Save()
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
		repos := m.cache.Repos()
		for _, r := range m.cfg.Repos {
			if !m.scheduledRepos[r] {
				repos = append(repos, r)
			}
		}
		cmds := make([]tea.Cmd, 0, len(repos))
		for i, repo := range repos {
			delay := time.Duration(i) * 200 * time.Millisecond
			cmds = append(cmds, scheduledRepoRefreshCmd(repo, delay))
		}
		return m, tea.Batch(cmds...)

	case key.Matches(msg, keys.Enter):
		if pr, ok := m.list.Selected(); ok {
			m.detail = detail.New(pr).SetSize(m.width, m.height-1)
			m.current = viewDetail
			return m, nil
		}
		// Cursor is on a header row: let prlist handle it (toggle collapse).
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd

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
				prepareFixCICmd(pr, m.cfg),
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

	case key.Matches(msg, keys.ClearCache):
		return m.startConfirm("Clear cache and re-fetch all? (y/n)",
			func() tea.Msg { return cacheClearedMsg{} }), nil

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
	bodyH := m.height - 1
	body = lipgloss.NewStyle().Height(bodyH).MaxHeight(bodyH).Render(body)
	content := lipgloss.JoinVertical(lipgloss.Left, body, bottom)

	switch {
	case m.confirming:
		content = m.renderPopup()
	case m.current == viewError:
		content = m.renderErrorPopup()
	case m.loading && m.lastFetch == 0 && len(m.cache.AllPRs()) == 0:
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

// computeLoadingRepos returns repos that are scheduled for refresh but have no
// cache data yet. These show as spinner rows in the list from startup.
func computeLoadingRepos(c *cache.Cache, scheduledRepos map[string]bool) map[string]bool {
	cached := make(map[string]bool, len(c.Repos()))
	for _, r := range c.Repos() {
		cached[r] = true
	}
	loading := make(map[string]bool)
	for r := range scheduledRepos {
		if !cached[r] {
			loading[r] = true
		}
	}
	return loading
}

// computeSpinningRepos returns the set of repos that should show the spinner +
// dim style: repos with an in-flight fetch OR repos with a stale cache entry.
func computeSpinningRepos(c *cache.Cache, cfg *config.Config, fetching map[string]bool) map[string]bool {
	spinning := make(map[string]bool)
	for _, repo := range c.Repos() {
		if c.IsStale(repo, cfg.CacheMaxAge) {
			spinning[repo] = true
		}
	}
	for repo := range fetching {
		spinning[repo] = true
	}
	return spinning
}

// initialRepoDelay computes how long to wait before fetching repo on startup.
// Stale or unknown repos refresh soon (small stagger). Fresh repos wait out
// their remaining interval plus a random jitter.
func initialRepoDelay(c *cache.Cache, repo string, cfg *config.Config, i, n int) time.Duration {
	entry, ok := c.Get(repo)
	if !ok || c.IsStale(repo, cfg.CacheMaxAge) {
		if n <= 1 {
			return 0
		}
		slot := cfg.RefreshInterval / time.Duration(n*4)
		return time.Duration(i) * slot
	}
	age := time.Since(entry.FetchedAt)
	remaining := cfg.RefreshInterval - age
	if remaining < 0 {
		remaining = 0
	}
	jitter := safeJitter(cfg.RefreshInterval / 5)
	return remaining + jitter
}

// safeJitter returns a random duration in [0, bound). If bound <= 0, returns 0.
func safeJitter(bound time.Duration) time.Duration {
	if bound <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(bound)))
}

// randomJitter returns a stagger delay for slot i of n, spread across interval.
func randomJitter(i, n int, interval time.Duration) time.Duration {
	denom := n
	if denom < 1 {
		denom = 1
	}
	return time.Duration(i)*interval/time.Duration(denom) +
		time.Duration(rand.Int63n(int64(interval/time.Duration(denom+1)+1)))
}

func formatAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/24/365))
	}
}

func truncateStyled(hints []string, sep string, maxWidth int) string {
	ellipsis := styleDim.Render(" …")
	for n := len(hints); n > 0; n-- {
		line := strings.Join(hints[:n], sep)
		if n == len(hints) && lipgloss.Width(line) <= maxWidth {
			return line
		}
		if lipgloss.Width(line)+lipgloss.Width(ellipsis) <= maxWidth {
			return line + ellipsis
		}
	}
	return styleDim.Render("…")
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
			helpHint("z", "collapse"),
			helpHint("/", "filter"),
			helpHint("o", "open"),
			helpHint("!", "auto"),
			helpHint("X", "clear cache"),
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
		if m.lastFetch == 0 {
			status = styleDim.Render("↻ syncing…")
		} else {
			ago := time.Since(time.Unix(0, m.lastFetch))
			status = styleDim.Render("↻ " + formatAgo(ago) + " ago")
		}
	}

	pad := 2
	innerW := m.width - 2*pad
	maxHelp := innerW - lipgloss.Width(status) - 2
	if maxHelp > 0 && lipgloss.Width(helpLine) > maxHelp {
		helpLine = truncateStyled(hints, sep, maxHelp)
	}
	gap := max(2, innerW-lipgloss.Width(helpLine)-lipgloss.Width(status))
	lr := strings.Repeat(" ", pad)

	return styleBottomBar.Render(lr + helpLine + strings.Repeat(" ", gap) + status + lr)
}
