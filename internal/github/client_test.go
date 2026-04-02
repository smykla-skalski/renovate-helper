package github

import (
	"strings"
	"testing"
	"time"

	"github.com/klaudiush/gh-renovate-tracker/internal/config"
)

func TestBuildSearchQuery_OrgsOnly(t *testing.T) {
	cfg := &config.Config{
		Orgs:   []string{"kumahq", "kong"},
		Author: "renovate[bot]",
	}
	query, aliases := buildSearchQuery(cfg)

	if len(aliases) != 2 {
		t.Fatalf("aliases len = %d, want 2", len(aliases))
	}
	if aliases[0] != "org0" || aliases[1] != "org1" {
		t.Errorf("aliases = %v, want [org0 org1]", aliases)
	}
	if !strings.Contains(query, "org:kumahq") {
		t.Error("query missing org:kumahq")
	}
	if !strings.Contains(query, "org:kong") {
		t.Error("query missing org:kong")
	}
}

func TestBuildSearchQuery_ReposOnly(t *testing.T) {
	cfg := &config.Config{
		Repos:  []string{"Kong/kong-mesh"},
		Author: "renovate[bot]",
	}
	_, aliases := buildSearchQuery(cfg)

	if len(aliases) != 1 || aliases[0] != "repo0" {
		t.Errorf("aliases = %v, want [repo0]", aliases)
	}
}

func TestBuildSearchQuery_Mixed(t *testing.T) {
	cfg := &config.Config{
		Orgs:   []string{"kumahq"},
		Repos:  []string{"Kong/kong-mesh"},
		Author: "renovate[bot]",
	}
	_, aliases := buildSearchQuery(cfg)

	if len(aliases) != 2 {
		t.Fatalf("aliases len = %d, want 2", len(aliases))
	}
	if aliases[0] != "org0" || aliases[1] != "repo0" {
		t.Errorf("aliases = %v, want [org0 repo0]", aliases)
	}
}

func TestConvertNode_ReviewDecision(t *testing.T) {
	cases := []struct {
		decision string
		want     string
	}{
		{"APPROVED", "APPROVED"},
		{"CHANGES_REQUESTED", "CHANGES_REQUESTED"},
		{"REVIEW_REQUIRED", "REVIEW_REQUIRED"},
		{"", ""},
	}
	for _, tc := range cases {
		n := prNode{ReviewDecision: tc.decision}
		pr := convertNode(n)
		if pr.ReviewStatus != tc.want {
			t.Errorf("decision %q: ReviewStatus = %q, want %q", tc.decision, pr.ReviewStatus, tc.want)
		}
	}
}

func makeCheckNode(status, conclusion string) prNode {
	var n prNode
	n.Commits.Nodes = []struct {
		Commit struct {
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []checkContext
				}
			}
		}
	}{{}}
	n.Commits.Nodes[0].Commit.StatusCheckRollup = &struct {
		Contexts struct {
			Nodes []checkContext
		}
	}{Contexts: struct {
		Nodes []checkContext
	}{Nodes: []checkContext{{Name: "ci", Status: status, Conclusion: conclusion}}}}
	return n
}

func TestConvertNode_CheckStatus(t *testing.T) {
	cases := []struct {
		status, conclusion, want string
	}{
		{"COMPLETED", "SUCCESS", "SUCCESS"},
		{"COMPLETED", "FAILURE", "FAILURE"},
		{"IN_PROGRESS", "", "PENDING"},
	}
	for _, tc := range cases {
		n := makeCheckNode(tc.status, tc.conclusion)
		pr := convertNode(n)
		if pr.CheckStatus != tc.want {
			t.Errorf("status=%q conclusion=%q: CheckStatus = %q, want %q",
				tc.status, tc.conclusion, pr.CheckStatus, tc.want)
		}
	}
}

func TestConvertNode_Labels(t *testing.T) {
	var n prNode
	n.Labels.Nodes = []struct{ Name string }{{Name: "automerge"}, {Name: "renovate"}}
	pr := convertNode(n)
	if len(pr.Labels) != 2 || pr.Labels[0] != "automerge" {
		t.Errorf("Labels = %v", pr.Labels)
	}
}

func TestConvertNode_Fields(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	n := prNode{
		ID:        "PR_id1",
		Number:    42,
		Title:     "chore(deps): update go",
		URL:       "https://github.com/org/repo/pull/42",
		State:     "OPEN",
		Mergeable: "MERGEABLE",
		Additions: 10,
		Deletions: 3,
		CreatedAt: now,
	}
	n.Repository.NameWithOwner = "org/repo"

	pr := convertNode(n)
	if pr.ID != "PR_id1" {
		t.Errorf("ID = %q", pr.ID)
	}
	if pr.Number != 42 {
		t.Errorf("Number = %d", pr.Number)
	}
	if pr.Repo != "org/repo" {
		t.Errorf("Repo = %q", pr.Repo)
	}
	if pr.Additions != 10 || pr.Deletions != 3 {
		t.Errorf("Additions/Deletions = %d/%d", pr.Additions, pr.Deletions)
	}
}
