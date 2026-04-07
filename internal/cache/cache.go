package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
)

// Entry holds the cached PRs for a single repository and when they were fetched.
type Entry struct {
	FetchedAt time.Time   `json:"fetched_at"`
	PRs       []github.PR `json:"prs"`
}

// Cache is a thread-safe, disk-backed store of per-repo PR snapshots.
// Reads and writes arrive from tea.Cmd goroutines so all access is guarded
// by a RWMutex.
type Cache struct {
	entries map[string]Entry // key: "owner/repo"
	path    string
	mu      sync.RWMutex
}

// defaultCachePath returns the OS-appropriate cache file location.
func defaultCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache dir: %w", err)
	}
	return filepath.Join(dir, "gh-renovate-tracker", "cache.json"), nil
}

// Load reads the cache from its default OS path. If the file does not exist
// an empty, usable Cache is returned with no error.
func Load() (*Cache, error) {
	path, err := defaultCachePath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads the cache from the given path. If the file does not exist an
// empty, usable Cache is returned with no error.
func LoadFrom(path string) (*Cache, error) {
	c := &Cache{path: path, entries: make(map[string]Entry)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}
	if err := json.Unmarshal(data, &c.entries); err != nil {
		return nil, fmt.Errorf("parse cache: %w", err)
	}
	return c, nil
}

// Empty returns an empty Cache using the default OS path.
// If the default path cannot be determined the path is left blank; Save will
// fail but all read/write operations on the in-memory state still work.
func Empty() *Cache {
	path, _ := defaultCachePath()
	return &Cache{path: path, entries: make(map[string]Entry)}
}

// EmptyAt returns an empty Cache using the given explicit path.
// Intended for testing.
func EmptyAt(path string) *Cache {
	return &Cache{path: path, entries: make(map[string]Entry)}
}

// Get returns the cached entry for the given repo and whether it was found.
func (c *Cache) Get(repo string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[repo]
	return e, ok
}

// Set stores or replaces the cached entry for repo.
func (c *Cache) Set(repo string, prs []github.PR, fetchedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[repo] = Entry{PRs: prs, FetchedAt: fetchedAt}
}

// Save persists the current cache to disk atomically (write temp file → rename).
// If the directory does not exist it is created.
func (c *Cache) Save() error {
	c.mu.RLock()
	data, err := json.Marshal(c.entries)
	c.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	dir := filepath.Dir(c.path)
	if errMkdir := os.MkdirAll(dir, 0o755); errMkdir != nil {
		return fmt.Errorf("mkdir cache dir: %w", errMkdir)
	}

	// Write to a sibling temp file then rename for atomicity.
	tmp, err := os.CreateTemp(dir, "cache-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp cache: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp cache: %w", err)
	}
	if err := os.Rename(tmpName, c.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}

// AllPRs returns a flat slice of all cached PRs across all repos.
func (c *Cache) AllPRs() []github.PR {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var total int
	for _, e := range c.entries {
		total += len(e.PRs)
	}
	prs := make([]github.PR, 0, total)
	for _, e := range c.entries {
		prs = append(prs, e.PRs...)
	}
	return prs
}

// Repos returns the names of all repos with a cache entry.
func (c *Cache) Repos() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	repos := make([]string, 0, len(c.entries))
	for repo := range c.entries {
		repos = append(repos, repo)
	}
	return repos
}

// IsStale reports whether the cached entry for repo is older than maxAge,
// or if maxAge is 0, or if no entry exists.
func (c *Cache) IsStale(repo string, maxAge time.Duration) bool {
	if maxAge == 0 {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[repo]
	if !ok {
		return true
	}
	return time.Since(e.FetchedAt) > maxAge
}
