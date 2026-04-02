package list

import (
	"strings"
	"testing"
	"time"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

func TestApplyFilter_Empty(t *testing.T) {
	prs := []github.PR{
		{Repo: "kumahq/kuma", Title: "update go"},
		{Repo: "Kong/mesh", Title: "update helm"},
	}
	got := applyFilter(prs, "")
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestApplyFilter_ByRepo(t *testing.T) {
	prs := []github.PR{
		{Repo: "kumahq/kuma", Title: "update go"},
		{Repo: "Kong/mesh", Title: "update helm"},
	}
	got := applyFilter(prs, "kumahq")
	if len(got) != 1 || got[0].Repo != "kumahq/kuma" {
		t.Errorf("got %v", got)
	}
}

func TestApplyFilter_ByTitle(t *testing.T) {
	prs := []github.PR{
		{Repo: "org/a", Title: "update go toolchain"},
		{Repo: "org/b", Title: "update helm chart"},
	}
	got := applyFilter(prs, "helm")
	if len(got) != 1 || got[0].Repo != "org/b" {
		t.Errorf("got %v", got)
	}
}

func TestApplyFilter_CaseInsensitive(t *testing.T) {
	prs := []github.PR{{Repo: "Org/Repo", Title: "Update Go"}}
	got := applyFilter(prs, "org")
	if len(got) != 1 {
		t.Error("case-insensitive filter failed")
	}
}

func TestStatusWeight_Order(t *testing.T) {
	ready := github.PR{ReviewStatus: "APPROVED", CheckStatus: "SUCCESS"}
	pending := github.PR{CheckStatus: "PENDING"}
	reviewNeeded := github.PR{ReviewStatus: "REVIEW_REQUIRED"}
	failed := github.PR{CheckStatus: "FAILURE"}
	conflict := github.PR{Mergeable: "CONFLICTING"}

	if statusWeight(ready) >= statusWeight(pending) {
		t.Error("ready should rank above pending")
	}
	if statusWeight(pending) >= statusWeight(reviewNeeded) {
		t.Error("pending should rank above review-required")
	}
	if statusWeight(reviewNeeded) >= statusWeight(failed) {
		t.Error("review-required should rank above failed")
	}
	if statusWeight(failed) >= statusWeight(conflict) {
		t.Error("failed should rank above conflict")
	}
}

func TestSortPRs_ByStatus(t *testing.T) {
	prs := []github.PR{
		{Repo: "a", CheckStatus: "FAILURE"},
		{Repo: "b", ReviewStatus: "APPROVED", CheckStatus: "SUCCESS"},
		{Repo: "c", CheckStatus: "PENDING"},
	}
	sortPRs(prs, sortByStatus)
	if prs[0].Repo != "b" {
		t.Errorf("first = %q, want b (ready)", prs[0].Repo)
	}
	if prs[2].Repo != "a" {
		t.Errorf("last = %q, want a (failed)", prs[2].Repo)
	}
}

func TestSortPRs_ByRepo(t *testing.T) {
	prs := []github.PR{
		{Repo: "zzz/z"},
		{Repo: "aaa/a"},
		{Repo: "mmm/m"},
	}
	sortPRs(prs, sortByRepo)
	if prs[0].Repo != "aaa/a" {
		t.Errorf("first = %q, want aaa/a", prs[0].Repo)
	}
}

func TestSortPRs_ByAge(t *testing.T) {
	now := time.Now()
	prs := []github.PR{
		{Repo: "old", CreatedAt: now.Add(-48 * time.Hour)},
		{Repo: "new", CreatedAt: now.Add(-1 * time.Hour)},
		{Repo: "mid", CreatedAt: now.Add(-24 * time.Hour)},
	}
	sortPRs(prs, sortByAge)
	if prs[0].Repo != "new" {
		t.Errorf("first = %q, want new", prs[0].Repo)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short = %q", got)
	}
	got := truncate("hello world this is long", 10)
	if len([]rune(got)) > 10 {
		t.Errorf("truncate did not shorten: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncate missing ellipsis: %q", got)
	}
}

func TestMax(t *testing.T) {
	if max(3, 5) != 5 {
		t.Error("max(3,5) != 5")
	}
	if max(5, 3) != 5 {
		t.Error("max(5,3) != 5")
	}
	if max(0, 0) != 0 {
		t.Error("max(0,0) != 0")
	}
}

func TestPrAge(t *testing.T) {
	if got := prAge(time.Now().Add(-30 * time.Minute)); !strings.HasSuffix(got, "m") {
		t.Errorf("30min ago = %q, want suffix m", got)
	}
	if got := prAge(time.Now().Add(-3 * time.Hour)); !strings.HasSuffix(got, "h") {
		t.Errorf("3h ago = %q, want suffix h", got)
	}
	if got := prAge(time.Now().Add(-48 * time.Hour)); !strings.HasSuffix(got, "d") {
		t.Errorf("2d ago = %q, want suffix d", got)
	}
}

func TestModelSelectedEmpty(t *testing.T) {
	m := New()
	if _, ok := m.Selected(); ok {
		t.Error("Selected() on empty model should return false")
	}
}

func TestModelSetFilter(t *testing.T) {
	m := New().SetPRs([]github.PR{
		{Repo: "a/b", Title: "update go"},
		{Repo: "c/d", Title: "update helm"},
	})
	m = m.SetFilter("helm")
	if _, ok := m.Selected(); !ok {
		t.Fatal("no selection after filter")
	}
	pr, _ := m.Selected()
	if pr.Repo != "c/d" {
		t.Errorf("filtered selected = %q, want c/d", pr.Repo)
	}
}
