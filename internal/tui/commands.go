package tui

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/smykla-skalski/gh-renovate-helper/internal/config"
	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
)

// prFetcher is the subset of github.Client used by per-repo and org-discovery
// commands. Defined as an interface to enable testing without network access.
type prFetcher interface {
	FetchRepoPRs(repo string, cfg *config.Config) ([]github.PR, error)
	FetchScopePRs(scope string, cfg *config.Config) ([]github.PR, error)
}

type (
	prsLoadedMsg  struct{ prs []github.PR }
	errMsg        struct{ err error }
	actionDoneMsg struct {
		msg  string
		repo string
	}
	batchProgressMsg struct {
		ch    <-chan tea.Msg
		cur   string // e.g. "owner/repo#123"
		verb  string
		done  int
		total int
	}
	fixCIReadyMsg struct {
		worktreeDir string
		prompt      string
		prKey       string
	}
	fixCIDoneMsg struct {
		err   error
		dir   string
		prKey string
	}
	clipboardDoneMsg struct{ count int }
	repoPRsLoadedMsg struct {
		fetchedAt time.Time
		repo      string
		prs       []github.PR
	}
	orgDiscoveredMsg struct {
		fetchedAt time.Time
		reposPRs  map[string][]github.PR
		org       string
	}
	autoModeDoneMsg struct {
		repos    []string
		approved int
		merged   int
	}
)

func fetchPRsCmd(client *github.Client, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		prs, err := client.FetchPRs(cfg)
		if err != nil {
			return errMsg{err}
		}
		return prsLoadedMsg{prs}
	}
}

func fetchRepoPRsCmd(client *github.Client, cfg *config.Config, repo string) tea.Cmd {
	return fetchRepoPRsCmdWith(client, cfg, repo)
}

// fetchRepoPRsCmdWith is the testable variant that accepts a prFetcher interface.
func fetchRepoPRsCmdWith(f prFetcher, cfg *config.Config, repo string) tea.Cmd {
	return func() tea.Msg {
		prs, err := f.FetchRepoPRs(repo, cfg)
		if err != nil {
			return errMsg{err}
		}
		return repoPRsLoadedMsg{repo: repo, prs: prs, fetchedAt: time.Now()}
	}
}

// scheduledRepoRefreshCmd sleeps `after`, then fetches a single repo's PRs.
func scheduledRepoRefreshCmd(f prFetcher, cfg *config.Config, repo string, after time.Duration) tea.Cmd {
	return func() tea.Msg {
		if after > 0 {
			time.Sleep(after)
		}
		prs, err := f.FetchRepoPRs(repo, cfg)
		if err != nil {
			return errMsg{err: fmt.Errorf("refresh %s: %w", repo, err)}
		}
		return repoPRsLoadedMsg{repo: repo, prs: prs, fetchedAt: time.Now()}
	}
}

// scheduledOrgDiscoverCmd sleeps `after`, then runs an org-level query to
// discover which repos have open PRs and groups the results by repo.
func scheduledOrgDiscoverCmd(f prFetcher, cfg *config.Config, org string, after time.Duration) tea.Cmd {
	return func() tea.Msg {
		if after > 0 {
			time.Sleep(after)
		}
		prs, err := f.FetchScopePRs("org:"+org, cfg)
		if err != nil {
			return errMsg{err: fmt.Errorf("discover org %s: %w", org, err)}
		}
		reposPRs := make(map[string][]github.PR)
		for _, pr := range prs {
			reposPRs[pr.Repo] = append(reposPRs[pr.Repo], pr)
		}
		return orgDiscoveredMsg{org: org, reposPRs: reposPRs, fetchedAt: time.Now()}
	}
}

func mergePRCmd(client *github.Client, pr github.PR, method string) tea.Cmd {
	return func() tea.Msg {
		if err := client.MergePR(pr.ID, method); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Merged " + pr.Repo + "#" + strconv.Itoa(pr.Number), repo: pr.Repo}
	}
}

func approvePRCmd(client *github.Client, pr github.PR) tea.Cmd {
	return func() tea.Msg {
		if err := client.ApprovePR(pr.ID); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Approved " + pr.Repo + "#" + strconv.Itoa(pr.Number), repo: pr.Repo}
	}
}

func addLabelCmd(client *github.Client, pr github.PR, label string) tea.Cmd {
	return func() tea.Msg {
		parts := strings.SplitN(pr.Repo, "/", 2)
		if len(parts) != 2 {
			return errMsg{err: fmt.Errorf("invalid repo: %s", pr.Repo)}
		}
		if err := client.AddLabel(parts[0], parts[1], pr.Number, label); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: fmt.Sprintf("Added label %q to %s#%d", label, pr.Repo, pr.Number), repo: pr.Repo}
	}
}

const batchMaxConcurrency = 3

func runBatch(prs []github.PR, verb string, fn func(github.PR) error, progressCh chan tea.Msg) tea.Msg {
	slog.Info("batch start", "verb", verb, "count", len(prs))
	errs := make([]error, len(prs))
	var done atomic.Int32
	total := len(prs)
	sem := make(chan struct{}, batchMaxConcurrency)
	var wg sync.WaitGroup
	for i := range prs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = fn(prs[i])
			n := int(done.Add(1))
			progressCh <- batchProgressMsg{
				ch:    progressCh,
				done:  n,
				total: total,
				verb:  verb,
				cur:   fmt.Sprintf("%s#%d", prs[i].Repo, prs[i].Number),
			}
		}(i)
	}
	wg.Wait()
	close(progressCh)
	var count int
	for i, err := range errs {
		if err != nil {
			return errMsg{err: fmt.Errorf("%s %s#%d: %w", verb, prs[i].Repo, prs[i].Number, err)}
		}
		count++
	}
	past := strings.ToUpper(verb[:1]) + verb[1:] + "d"
	slog.Info("batch complete", "verb", verb, "count", count)
	repo := ""
	if len(prs) > 0 {
		repo = prs[0].Repo
	}
	return actionDoneMsg{msg: fmt.Sprintf("%s %d PRs", past, count), repo: repo}
}

func listenProgress(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func batchMergeCmd(client *github.Client, prs []github.PR, method string) tea.Cmd {
	ch := make(chan tea.Msg, len(prs))
	return tea.Batch(
		func() tea.Msg {
			return runBatch(prs, "merge", func(pr github.PR) error {
				return client.MergePR(pr.ID, method)
			}, ch)
		},
		listenProgress(ch),
	)
}

func batchApproveCmd(client *github.Client, prs []github.PR) tea.Cmd {
	ch := make(chan tea.Msg, len(prs))
	return tea.Batch(
		func() tea.Msg {
			return runBatch(prs, "approve", func(pr github.PR) error {
				return client.ApprovePR(pr.ID)
			}, ch)
		},
		listenProgress(ch),
	)
}

func autoModeCmd(client *github.Client, toApprove, toMerge []github.PR, method string) tea.Cmd {
	return func() tea.Msg {
		// Phase 1: approve.
		for i := range toApprove {
			if err := client.ApprovePR(toApprove[i].ID); err != nil {
				slog.Error("auto-approve failed", "pr", toApprove[i].Repo, "num", toApprove[i].Number, "err", err)
				return errMsg{err: fmt.Errorf("auto-approve %s#%d: %w", toApprove[i].Repo, toApprove[i].Number, err)}
			}
			slog.Info("auto-approved", "pr", toApprove[i].Repo, "num", toApprove[i].Number)
		}

		// Phase 2: merge (already-approved + just-approved).
		allMerge := make([]github.PR, 0, len(toMerge)+len(toApprove))
		allMerge = append(allMerge, toMerge...)
		allMerge = append(allMerge, toApprove...)
		for i := range allMerge {
			if err := client.MergePR(allMerge[i].ID, method); err != nil {
				slog.Error("auto-merge failed", "pr", allMerge[i].Repo, "num", allMerge[i].Number, "err", err)
				return errMsg{err: fmt.Errorf("auto-merge %s#%d: %w", allMerge[i].Repo, allMerge[i].Number, err)}
			}
			slog.Info("auto-merged", "pr", allMerge[i].Repo, "num", allMerge[i].Number)
		}

		seen := make(map[string]bool)
		for i := range allMerge {
			seen[allMerge[i].Repo] = true
		}
		for i := range toApprove {
			seen[toApprove[i].Repo] = true
		}
		repos := make([]string, 0, len(seen))
		for r := range seen {
			repos = append(repos, r)
		}
		return autoModeDoneMsg{approved: len(toApprove), merged: len(allMerge), repos: repos}
	}
}

func rerunChecksCmd(client *github.Client, pr github.PR) tea.Cmd {
	return func() tea.Msg {
		parts := strings.SplitN(pr.Repo, "/", 2)
		if len(parts) != 2 {
			return errMsg{err: fmt.Errorf("invalid repo: %s", pr.Repo)}
		}
		var suiteIDs []string
		for _, cr := range pr.Checks {
			if cr.Conclusion == "FAILURE" || cr.Conclusion == "TIMED_OUT" {
				if cr.SuiteID != "" {
					suiteIDs = append(suiteIDs, cr.SuiteID)
				}
			}
		}
		if err := client.RerunChecks(parts[0], parts[1], suiteIDs); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Rerun checks for " + pr.Repo + "#" + strconv.Itoa(pr.Number), repo: pr.Repo}
	}
}

const (
	bareRepoBase = "/tmp/renovate-helper-repos"
	worktreeBase = "/tmp/renovate-helper-worktrees"
)

func parsePRRepo(repo string) (owner, name string, err error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo: %s", repo)
	}
	return parts[0], parts[1], nil
}

func worktreePaths(owner, repo string, number int) (bareDir, wtDir string) {
	bareDir = filepath.Join(bareRepoBase, owner, repo)
	wtDir = filepath.Join(worktreeBase, fmt.Sprintf("%s-%s-pr-%d", owner, repo, number))
	return bareDir, wtDir
}

// cloneBareRepo creates a bare clone of owner/repo at bareDir using a gh auth token.
func cloneBareRepo(ctx context.Context, owner, repo, bareDir string) error {
	if err := os.MkdirAll(filepath.Dir(bareDir), 0o755); err != nil {
		return fmt.Errorf("mkdir bare: %w", err)
	}
	tokenOut, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return fmt.Errorf("gh auth token: %w", err)
	}
	token := strings.TrimSpace(string(tokenOut))
	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)
	slog.Info("cloning bare repo", "repo", owner+"/"+repo, "dir", bareDir)
	if out, err := exec.CommandContext(ctx, "git", "clone", "--bare", cloneURL, bareDir).CombinedOutput(); err != nil {
		return fmt.Errorf("git clone --bare: %s: %w", out, err)
	}
	return nil
}

func buildFixCIPrompt(pr github.PR) string {
	return fmt.Sprintf(`Fix CI failures on this Renovate dependency update PR: %s

- Analyze failures, fix code so CI passes
- Make minimal targeted changes
- Run failing checks locally to verify
- Commit fixes with -s -S flags`, pr.URL)
}

// resolveRemote returns the remote name to use for the bare repo at bareDir.
// Priority: cfg.Remote (explicit) > "upstream" (if present) > "origin" (if present).
// Returns an error if no usable remote is found.
func resolveRemote(ctx context.Context, bareDir string, cfg *config.Config) (string, error) {
	if cfg.Remote != "" {
		return cfg.Remote, nil
	}
	out, err := exec.CommandContext(ctx, "git", "-C", bareDir, "remote").Output()
	if err != nil {
		return "", fmt.Errorf("git remote: %w", err)
	}
	remotes := strings.Fields(string(out))
	set := make(map[string]bool, len(remotes))
	for _, r := range remotes {
		set[r] = true
	}
	if set["upstream"] {
		return "upstream", nil
	}
	if set["origin"] {
		return "origin", nil
	}
	return "", fmt.Errorf("no usable remote found in %s (checked upstream, origin)", bareDir)
}

func prepareFixCICmd(pr github.PR, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		owner, repo, err := parsePRRepo(pr.Repo)
		if err != nil {
			return errMsg{err: err}
		}

		prKey := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)

		// Get PR branch name.
		branchCmd := exec.CommandContext(ctx, "gh", "pr", "view",
			strconv.Itoa(pr.Number),
			"--repo", pr.Repo,
			"--json", "headRefName",
			"-q", ".headRefName",
		)
		var branchStderr bytes.Buffer
		branchCmd.Stderr = &branchStderr
		branchOut, err := branchCmd.Output()
		if err != nil {
			return errMsg{err: fmt.Errorf("get PR branch: %w: %s", err, strings.TrimSpace(branchStderr.String()))}
		}
		branch := strings.TrimSpace(string(branchOut))

		bareDir, wtDir := worktreePaths(owner, repo, pr.Number)

		// Setup bare clone if needed.
		if _, statErr := os.Stat(bareDir); os.IsNotExist(statErr) {
			if err = cloneBareRepo(ctx, owner, repo, bareDir); err != nil {
				return errMsg{err: err}
			}
		}

		remote, err := resolveRemote(ctx, bareDir, cfg)
		if err != nil {
			return errMsg{err: err}
		}

		// Fetch the branch. In a bare repo the default refspec maps to
		// refs/heads/*, not refs/remotes/<remote>/*, so the fetched branch
		// lands at refs/heads/<branch> - use that ref for worktree add.
		slog.Info("fetching branch", "repo", pr.Repo, "branch", branch, "remote", remote)
		if out, err := exec.CommandContext(ctx, "git", "-C", bareDir, "fetch", remote, branch).CombinedOutput(); err != nil {
			return errMsg{err: fmt.Errorf("git fetch: %s: %w", out, err)}
		}

		// Clean existing worktree.
		if _, err := os.Stat(wtDir); err == nil {
			_ = exec.CommandContext(ctx, "git", "-C", bareDir, "worktree", "remove", "--force", wtDir).Run()
			_ = os.RemoveAll(wtDir)
		}

		// Create worktree using the local ref (not remote/branch which
		// doesn't exist in bare repos - there are no remote-tracking refs).
		slog.Info("creating worktree", "dir", wtDir, "branch", branch)
		if out, err := exec.CommandContext(ctx, "git", "-C", bareDir, "worktree", "add", wtDir, branch).CombinedOutput(); err != nil {
			return errMsg{err: fmt.Errorf("git worktree add: %s: %w", out, err)}
		}

		prompt := buildFixCIPrompt(pr)
		return fixCIReadyMsg{worktreeDir: wtDir, prompt: prompt, prKey: prKey}
	}
}

func fixCIExecCmd(dir, prompt, prKey string) tea.Cmd {
	c := exec.CommandContext(context.Background(), "claude", "-p", prompt, "--dangerously-skip-permissions")
	c.Dir = dir
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return fixCIDoneMsg{prKey: prKey, err: err}
		}
		return fixCIDoneMsg{prKey: prKey, dir: dir}
	})
}
