package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Author != "renovate[bot]" {
		t.Errorf("Author = %q, want %q", cfg.Author, "renovate[bot]")
	}
	if cfg.MergeMethod != "squash" {
		t.Errorf("MergeMethod = %q, want %q", cfg.MergeMethod, "squash")
	}
	if cfg.RefreshInterval != 5*time.Minute {
		t.Errorf("RefreshInterval = %v, want %v", cfg.RefreshInterval, 5*time.Minute)
	}
}

func TestLoadFromFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "gh-renovate-tracker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `
orgs:
  - kumahq
repos:
  - Kong/kong-mesh
author: dependabot[bot]
merge_method: merge
refresh_interval: 10m
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Orgs) != 1 || cfg.Orgs[0] != "kumahq" {
		t.Errorf("Orgs = %v, want [kumahq]", cfg.Orgs)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0] != "Kong/kong-mesh" {
		t.Errorf("Repos = %v, want [Kong/kong-mesh]", cfg.Repos)
	}
	if cfg.Author != "dependabot[bot]" {
		t.Errorf("Author = %q, want dependabot[bot]", cfg.Author)
	}
	if cfg.MergeMethod != "merge" {
		t.Errorf("MergeMethod = %q, want merge", cfg.MergeMethod)
	}
	if cfg.RefreshInterval != 10*time.Minute {
		t.Errorf("RefreshInterval = %v, want 10m", cfg.RefreshInterval)
	}
}

func TestEmptyAuthorFallsBackToDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "gh-renovate-tracker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("orgs:\n  - foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Author != "renovate[bot]" {
		t.Errorf("Author = %q, want renovate[bot]", cfg.Author)
	}
}
