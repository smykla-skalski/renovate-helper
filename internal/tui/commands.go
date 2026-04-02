package tui

import (
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

	tea "charm.land/bubbletea/v2"

	"github.com/klaudiush/gh-renovate-tracker/internal/config"
	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

type (
	prsLoadedMsg     struct{ prs []github.PR }
	errMsg           struct{ err error }
	actionDoneMsg    struct{ msg string }
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

func mergePRCmd(client *github.Client, pr github.PR, method string) tea.Cmd {
	return func() tea.Msg {
		if err := client.MergePR(pr.ID, method); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Merged " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
	}
}

func approvePRCmd(client *github.Client, pr github.PR) tea.Cmd {
	return func() tea.Msg {
		if err := client.ApprovePR(pr.ID); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Approved " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
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
		return actionDoneMsg{msg: fmt.Sprintf("Added label %q to %s#%d", label, pr.Repo, pr.Number)}
	}
}

func runBatch(prs []github.PR, verb string, fn func(github.PR) error, progressCh chan tea.Msg) tea.Msg {
	slog.Info("batch start", "verb", verb, "count", len(prs))
	errs := make([]error, len(prs))
	var done atomic.Int32
	total := len(prs)
	var wg sync.WaitGroup
	for i := range prs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
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
	return actionDoneMsg{msg: fmt.Sprintf("%s %d PRs", past, count)}
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
		return actionDoneMsg{msg: "Rerun checks for " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
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

func buildFixCIPrompt(pr github.PR) string {
	return fmt.Sprintf(`Fix CI failures on this Renovate dependency update PR: %s

- Analyze failures, fix code so CI passes
- Make minimal targeted changes
- Run failing checks locally to verify
- Commit fixes with -s -S flags`, pr.URL)
}

func prepareFixCICmd(pr github.PR) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		owner, repo, err := parsePRRepo(pr.Repo)
		if err != nil {
			return errMsg{err: err}
		}

		prKey := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)

		// Get PR branch name.
		branchOut, err := exec.CommandContext(ctx, "gh", "pr", "view",
			strconv.Itoa(pr.Number),
			"--repo", pr.Repo,
			"--json", "headRefName",
			"-q", ".headRefName",
		).Output()
		if err != nil {
			return errMsg{err: fmt.Errorf("get PR branch: %w", err)}
		}
		branch := strings.TrimSpace(string(branchOut))

		bareDir, wtDir := worktreePaths(owner, repo, pr.Number)

		// Setup bare clone or fetch.
		if _, err := os.Stat(bareDir); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(bareDir), 0o755); err != nil {
				return errMsg{err: fmt.Errorf("mkdir bare: %w", err)}
			}
			tokenOut, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
			if err != nil {
				return errMsg{err: fmt.Errorf("gh auth token: %w", err)}
			}
			token := strings.TrimSpace(string(tokenOut))
			cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)
			slog.Info("cloning bare repo", "repo", pr.Repo, "dir", bareDir)
			if out, err := exec.CommandContext(ctx, "git", "clone", "--bare", cloneURL, bareDir).CombinedOutput(); err != nil {
				return errMsg{err: fmt.Errorf("git clone --bare: %s: %w", out, err)}
			}
		} else {
			slog.Info("fetching branch", "repo", pr.Repo, "branch", branch)
			if out, err := exec.CommandContext(ctx, "git", "-C", bareDir, "fetch", "origin", branch).CombinedOutput(); err != nil {
				return errMsg{err: fmt.Errorf("git fetch: %s: %w", out, err)}
			}
		}

		// Clean existing worktree.
		if _, err := os.Stat(wtDir); err == nil {
			_ = exec.CommandContext(ctx, "git", "-C", bareDir, "worktree", "remove", "--force", wtDir).Run()
			_ = os.RemoveAll(wtDir)
		}

		// Create worktree.
		slog.Info("creating worktree", "dir", wtDir, "branch", branch)
		if out, err := exec.CommandContext(ctx, "git", "-C", bareDir, "worktree", "add", wtDir, "origin/"+branch).CombinedOutput(); err != nil {
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
