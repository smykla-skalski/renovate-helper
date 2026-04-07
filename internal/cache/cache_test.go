package cache_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smykla-skalski/gh-renovate-helper/internal/cache"
	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
)

func makePRs(repo string, count int) []github.PR {
	prs := make([]github.PR, count)
	for i := range prs {
		prs[i] = github.PR{
			ID:    repo + "-" + string(rune('A'+i)),
			Repo:  repo,
			Title: "PR " + string(rune('A'+i)),
		}
	}
	return prs
}

// newTempCache writes JSON to a temp file and returns a *Cache loaded from it.
func writeCacheFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeCacheFile: %v", err)
	}
	return path
}

func TestLoad_NoFile(t *testing.T) {
	c := cache.Empty()
	if c == nil {
		t.Fatal("Empty() returned nil")
	}
	repos := c.Repos()
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	prs := makePRs("owner/repo", 2)
	type jsonEntry struct {
		FetchedAt time.Time   `json:"fetched_at"`
		PRs       []github.PR `json:"prs"`
	}
	data, err := json.Marshal(map[string]jsonEntry{
		"owner/repo": {PRs: prs, FetchedAt: now},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	path := writeCacheFile(t, string(data))
	c, err := cache.LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	entry, ok := c.Get("owner/repo")
	if !ok {
		t.Fatal("expected entry for owner/repo")
	}
	if len(entry.PRs) != 2 {
		t.Errorf("expected 2 PRs, got %d", len(entry.PRs))
	}
	if !entry.FetchedAt.Equal(now) {
		t.Errorf("FetchedAt: got %v, want %v", entry.FetchedAt, now)
	}
}

func TestLoad_CorruptJSON(t *testing.T) {
	path := writeCacheFile(t, "{bad json")
	_, err := cache.LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestSetGet_RoundTrip(t *testing.T) {
	c := cache.Empty()
	prs := makePRs("org/foo", 3)
	now := time.Now().UTC()

	c.Set("org/foo", prs, now)

	entry, ok := c.Get("org/foo")
	if !ok {
		t.Fatal("expected entry after Set")
	}
	if len(entry.PRs) != 3 {
		t.Errorf("expected 3 PRs, got %d", len(entry.PRs))
	}
}

func TestSet_Overwrites(t *testing.T) {
	c := cache.Empty()
	c.Set("org/bar", makePRs("org/bar", 5), time.Now())
	c.Set("org/bar", makePRs("org/bar", 2), time.Now())

	entry, ok := c.Get("org/bar")
	if !ok {
		t.Fatal("missing entry")
	}
	if len(entry.PRs) != 2 {
		t.Errorf("expected 2 PRs after overwrite, got %d", len(entry.PRs))
	}
}

func TestGet_Missing(t *testing.T) {
	c := cache.Empty()
	_, ok := c.Get("nobody/nowhere")
	if ok {
		t.Error("expected ok=false for missing repo")
	}
}

func TestSave_WritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "cache.json")
	c := cache.EmptyAt(path)
	c.Set("owner/repo", makePRs("owner/repo", 1), time.Now())

	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal saved JSON: %v", err)
	}
	if _, ok := raw["owner/repo"]; !ok {
		t.Error("saved JSON missing owner/repo key")
	}
}

func TestSave_Atomic(t *testing.T) {
	// A hard kill between write and rename would leave temp file, not corrupt cache.
	// We verify the final file is always valid JSON even if we write twice.
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	c := cache.EmptyAt(path)
	c.Set("a/b", makePRs("a/b", 1), time.Now())
	if err := c.Save(); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	c.Set("c/d", makePRs("c/d", 1), time.Now())
	if err := c.Save(); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	data, _ := os.ReadFile(path)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("corrupt after second save: %v", err)
	}
	if len(raw) != 2 {
		t.Errorf("expected 2 keys, got %d", len(raw))
	}
}

func TestSave_ConcurrentWithSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	c := cache.EmptyAt(path)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			repo := "org/repo-" + string(rune('a'+i))
			c.Set(repo, makePRs(repo, 1), time.Now())
			_ = c.Save()
		}(i)
	}
	wg.Wait()
	// If there's a data race the race detector will catch it.
}

func TestAllPRs(t *testing.T) {
	c := cache.Empty()
	c.Set("a/x", makePRs("a/x", 2), time.Now())
	c.Set("b/y", makePRs("b/y", 3), time.Now())

	all := c.AllPRs()
	if len(all) != 5 {
		t.Errorf("expected 5 PRs, got %d", len(all))
	}
}

func TestAllPRs_Empty(t *testing.T) {
	c := cache.Empty()
	if prs := c.AllPRs(); len(prs) != 0 {
		t.Errorf("expected 0 PRs on empty cache, got %d", len(prs))
	}
}

func TestRepos(t *testing.T) {
	c := cache.Empty()
	c.Set("x/a", nil, time.Now())
	c.Set("x/b", nil, time.Now())
	c.Set("x/c", nil, time.Now())

	repos := c.Repos()
	if len(repos) != 3 {
		t.Errorf("expected 3 repos, got %d", len(repos))
	}
}

func TestIsStale_ZeroMaxAge(t *testing.T) {
	c := cache.Empty()
	c.Set("r/x", nil, time.Now())
	if !c.IsStale("r/x", 0) {
		t.Error("expected IsStale=true with maxAge=0")
	}
}

func TestIsStale_Recent(t *testing.T) {
	c := cache.Empty()
	c.Set("r/x", nil, time.Now())
	if c.IsStale("r/x", 24*time.Hour) {
		t.Error("expected IsStale=false for freshly-set entry with 24h maxAge")
	}
}

func TestIsStale_Old(t *testing.T) {
	c := cache.Empty()
	c.Set("r/x", nil, time.Now().Add(-25*time.Hour))
	if !c.IsStale("r/x", 24*time.Hour) {
		t.Error("expected IsStale=true for 25h-old entry with 24h maxAge")
	}
}

func TestIsStale_UnknownRepo(t *testing.T) {
	c := cache.Empty()
	if !c.IsStale("nobody/nope", 24*time.Hour) {
		t.Error("expected IsStale=true for unknown repo")
	}
}

func TestEmpty_Usable(t *testing.T) {
	c := cache.Empty()
	c.Set("a/b", makePRs("a/b", 1), time.Now())
	if _, ok := c.Get("a/b"); !ok {
		t.Error("Empty cache should be usable after Set")
	}
}

func TestLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	c1 := cache.EmptyAt(path)
	now := time.Now().UTC().Truncate(time.Millisecond)
	prs := makePRs("rt/repo", 2)
	c1.Set("rt/repo", prs, now)
	if err := c1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	c2, err := cache.LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	entry, ok := c2.Get("rt/repo")
	if !ok {
		t.Fatal("expected entry after load")
	}
	if len(entry.PRs) != 2 {
		t.Errorf("expected 2 PRs, got %d", len(entry.PRs))
	}
	if entry.PRs[0].Repo != "rt/repo" {
		t.Errorf("PR.Repo: got %q", entry.PRs[0].Repo)
	}
}
