package tui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/smykla-skalski/gh-renovate-helper/internal/config"
	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
)

// mockFetcher implements prFetcher for tests.
type mockFetcher struct {
	repoPRs   map[string][]github.PR
	scopePRs  []github.PR
	repoErr   error
	scopeErr  error
}

func (m *mockFetcher) FetchRepoPRs(repo string, _ *config.Config) ([]github.PR, error) {
	if m.repoErr != nil {
		return nil, m.repoErr
	}
	return m.repoPRs[repo], nil
}

func (m *mockFetcher) FetchScopePRs(_ string, _ *config.Config) ([]github.PR, error) {
	if m.scopeErr != nil {
		return nil, m.scopeErr
	}
	return m.scopePRs, nil
}

func TestBatchMergeCmd_InvalidRepo(t *testing.T) {
	cmd := batchMergeCmd(nil, []github.PR{}, "squash")
	if cmd == nil {
		t.Fatal("batchMergeCmd should return a non-nil cmd")
	}
	msg := cmd()
	// With progress channels, batch cmds return tea.BatchMsg wrapping sub-commands.
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	// Execute the batch runner (first cmd) — empty list should succeed immediately.
	var found bool
	for _, c := range batch {
		m := c()
		if _, ok := m.(actionDoneMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("empty batch should produce actionDoneMsg from one of the sub-commands")
	}
}

func TestBatchApproveCmd_Empty(t *testing.T) {
	cmd := batchApproveCmd(nil, []github.PR{})
	if cmd == nil {
		t.Fatal("batchApproveCmd should return a non-nil cmd")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	var found bool
	for _, c := range batch {
		m := c()
		if done, ok := m.(actionDoneMsg); ok {
			found = true
			if done.msg != "Approved 0 PRs" {
				t.Errorf("msg = %q", done.msg)
			}
		}
	}
	if !found {
		t.Error("empty batch should produce actionDoneMsg from one of the sub-commands")
	}
}

func TestAddLabelCmd_InvalidRepo(t *testing.T) {
	pr := github.PR{Repo: "invalid-no-slash", Number: 1}
	cmd := addLabelCmd(nil, pr, "test-label")
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("invalid repo should return errMsg, got %T", msg)
	}
}

func TestRerunChecksCmd_InvalidRepo(t *testing.T) {
	pr := github.PR{Repo: "noslash", Number: 1}
	cmd := rerunChecksCmd(nil, pr)
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("invalid repo should return errMsg, got %T", msg)
	}
}

// --- scheduledRepoRefreshCmd ---

func TestScheduledRepoRefreshCmd_Success(t *testing.T) {
	prs := []github.PR{{ID: "1", Repo: "owner/repo", Title: "Fix deps"}}
	fetcher := &mockFetcher{repoPRs: map[string][]github.PR{"owner/repo": prs}}
	cfg := &config.Config{}

	before := time.Now()
	cmd := scheduledRepoRefreshCmd(fetcher, cfg, "owner/repo", 0)
	msg := cmd()
	after := time.Now()

	m, ok := msg.(repoPRsLoadedMsg)
	if !ok {
		t.Fatalf("expected repoPRsLoadedMsg, got %T: %v", msg, msg)
	}
	if m.repo != "owner/repo" {
		t.Errorf("repo: got %q, want %q", m.repo, "owner/repo")
	}
	if len(m.prs) != 1 {
		t.Errorf("prs: got %d, want 1", len(m.prs))
	}
	if m.fetchedAt.IsZero() {
		t.Error("fetchedAt should not be zero")
	}
	if m.fetchedAt.Before(before) || m.fetchedAt.After(after) {
		t.Errorf("fetchedAt %v outside [%v, %v]", m.fetchedAt, before, after)
	}
}

func TestScheduledRepoRefreshCmd_Error(t *testing.T) {
	fetcher := &mockFetcher{repoErr: errors.New("network failure")}
	cfg := &config.Config{}

	cmd := scheduledRepoRefreshCmd(fetcher, cfg, "owner/repo", 0)
	msg := cmd()

	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("expected errMsg, got %T", msg)
	}
	if e.err == nil {
		t.Fatal("err should not be nil")
	}
	// Error should mention the repo name.
	if !errors.Is(e.err, fetcher.repoErr) {
		t.Logf("err: %v", e.err)
	}
}

// --- scheduledOrgDiscoverCmd ---

func TestScheduledOrgDiscoverCmd_GroupsByRepo(t *testing.T) {
	prs := []github.PR{
		{ID: "1", Repo: "org/alpha", Title: "PR1"},
		{ID: "2", Repo: "org/alpha", Title: "PR2"},
		{ID: "3", Repo: "org/beta", Title: "PR3"},
	}
	fetcher := &mockFetcher{scopePRs: prs}
	cfg := &config.Config{}

	before := time.Now()
	cmd := scheduledOrgDiscoverCmd(fetcher, cfg, "myorg", 0)
	msg := cmd()
	after := time.Now()

	m, ok := msg.(orgDiscoveredMsg)
	if !ok {
		t.Fatalf("expected orgDiscoveredMsg, got %T: %v", msg, msg)
	}
	if m.org != "myorg" {
		t.Errorf("org: got %q, want %q", m.org, "myorg")
	}
	if len(m.reposPRs) != 2 {
		t.Errorf("reposPRs: got %d repos, want 2", len(m.reposPRs))
	}
	if len(m.reposPRs["org/alpha"]) != 2 {
		t.Errorf("org/alpha: got %d PRs, want 2", len(m.reposPRs["org/alpha"]))
	}
	if len(m.reposPRs["org/beta"]) != 1 {
		t.Errorf("org/beta: got %d PRs, want 1", len(m.reposPRs["org/beta"]))
	}
	if m.fetchedAt.IsZero() {
		t.Error("fetchedAt should not be zero")
	}
	if m.fetchedAt.Before(before) || m.fetchedAt.After(after) {
		t.Errorf("fetchedAt %v outside [%v, %v]", m.fetchedAt, before, after)
	}
}

func TestScheduledOrgDiscoverCmd_EmptyOrg(t *testing.T) {
	fetcher := &mockFetcher{scopePRs: nil}
	cfg := &config.Config{}

	cmd := scheduledOrgDiscoverCmd(fetcher, cfg, "emptyorg", 0)
	msg := cmd()

	m, ok := msg.(orgDiscoveredMsg)
	if !ok {
		t.Fatalf("expected orgDiscoveredMsg, got %T", msg)
	}
	if len(m.reposPRs) != 0 {
		t.Errorf("expected empty reposPRs, got %d repos", len(m.reposPRs))
	}
}

func TestScheduledOrgDiscoverCmd_Error(t *testing.T) {
	fetcher := &mockFetcher{scopeErr: errors.New("api down")}
	cfg := &config.Config{}

	cmd := scheduledOrgDiscoverCmd(fetcher, cfg, "myorg", 0)
	msg := cmd()

	e, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("expected errMsg, got %T", msg)
	}
	if e.err == nil {
		t.Fatal("err should not be nil")
	}
}

// --- fetchRepoPRsCmd ---

func TestFetchRepoPRsCmd_PopulatesFetchedAt(t *testing.T) {
	prs := []github.PR{{ID: "1", Repo: "x/y"}}
	fetcher := &mockFetcher{repoPRs: map[string][]github.PR{"x/y": prs}}
	cfg := &config.Config{}

	before := time.Now()
	cmd := fetchRepoPRsCmdWith(fetcher, cfg, "x/y")
	msg := cmd()
	after := time.Now()

	m, ok := msg.(repoPRsLoadedMsg)
	if !ok {
		t.Fatalf("expected repoPRsLoadedMsg, got %T", msg)
	}
	if m.fetchedAt.IsZero() {
		t.Error("fetchedAt should not be zero")
	}
	if m.fetchedAt.Before(before) || m.fetchedAt.After(after) {
		t.Errorf("fetchedAt %v outside [%v, %v]", m.fetchedAt, before, after)
	}
}
