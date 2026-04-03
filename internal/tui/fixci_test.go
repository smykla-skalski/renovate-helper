package tui

import (
	"strings"
	"testing"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

func TestParsePRRepo(t *testing.T) {
	tests := []struct {
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"kumahq/kuma", "kumahq", "kuma", false},
		{"kumahq/kuma-website", "kumahq", "kuma-website", false},
		{"invalid", "", "", true},
	}
	for _, tt := range tests {
		owner, repo, err := parsePRRepo(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parsePRRepo(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
		}
		if owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("parsePRRepo(%q) = (%q, %q), want (%q, %q)", tt.input, owner, repo, tt.wantOwner, tt.wantRepo)
		}
	}
}

func TestWorktreePaths(t *testing.T) {
	bareDir, wtDir := worktreePaths("kumahq", "kuma", 42)
	if bareDir != "/tmp/renovate-helper-repos/kumahq/kuma" {
		t.Errorf("bareDir = %q", bareDir)
	}
	if wtDir != "/tmp/renovate-helper-worktrees/kumahq-kuma-pr-42" {
		t.Errorf("wtDir = %q", wtDir)
	}
}

func TestBuildFixCIPrompt(t *testing.T) {
	pr := github.PR{
		URL:  "https://github.com/kumahq/kuma/pull/99",
		Repo: "kumahq/kuma",
	}
	prompt := buildFixCIPrompt(pr)
	if !strings.Contains(prompt, pr.URL) {
		t.Errorf("prompt should contain PR URL, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Fix CI") {
		t.Errorf("prompt should contain 'Fix CI', got: %s", prompt)
	}
}
