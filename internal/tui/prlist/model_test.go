package prlist

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

func TestApplyFilter_Empty(t *testing.T) {
	prs := []github.PR{
		{Repo: "kumahq/kuma", Title: "update go"},
		{Repo: "smykla-skalski/app", Title: "update helm"},
	}
	got := applyFilter(prs, "")
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestApplyFilter_ByRepo(t *testing.T) {
	prs := []github.PR{
		{Repo: "kumahq/kuma", Title: "update go"},
		{Repo: "smykla-skalski/app", Title: "update helm"},
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

func TestCell_Short(t *testing.T) {
	got := cell("hello", 10)
	if w := lipgloss.Width(got); w != 10 {
		t.Errorf("cell width = %d, want 10", w)
	}
}

func TestCell_Long(t *testing.T) {
	got := cell("hello world this is long", 10)
	if w := lipgloss.Width(got); w != 10 {
		t.Errorf("cell width = %d, want 10", w)
	}
	stripped := ansi.Strip(got)
	if len([]rune(stripped)) > 10 {
		t.Errorf("cell did not truncate: %q", stripped)
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

func TestSelectedPRs_Empty(t *testing.T) {
	m := New().SetPRs([]github.PR{
		{Repo: "a/b"},
		{Repo: "c/d"},
	})
	if got := m.SelectedPRs(); len(got) != 0 {
		t.Errorf("SelectedPRs() = %d, want 0", len(got))
	}
}

func TestSelectedPRs_MultiSelect(t *testing.T) {
	m := New().SetPRs([]github.PR{
		{Repo: "a/b"},
		{Repo: "c/d"},
		{Repo: "e/f"},
	})
	m.selected[0] = true
	m.selected[2] = true
	got := m.SelectedPRs()
	if len(got) != 2 {
		t.Fatalf("SelectedPRs() len = %d, want 2", len(got))
	}
	repos := map[string]bool{got[0].Repo: true, got[1].Repo: true}
	if !repos["a/b"] || !repos["e/f"] {
		t.Errorf("SelectedPRs repos = %v, want a/b and e/f", repos)
	}
}

func TestSelectedPRs_OutOfBounds(t *testing.T) {
	m := New().SetPRs([]github.PR{{Repo: "a/b"}})
	m.selected[5] = true
	if got := m.SelectedPRs(); len(got) != 0 {
		t.Errorf("SelectedPRs() with out-of-bounds index = %d, want 0", len(got))
	}
}

func TestClearSelected(t *testing.T) {
	m := New().SetPRs([]github.PR{{Repo: "a/b"}, {Repo: "c/d"}})
	m.selected[0] = true
	m.selected[1] = true
	m = m.ClearSelected()
	if got := m.SelectedPRs(); len(got) != 0 {
		t.Errorf("after ClearSelected(), SelectedPRs() = %d, want 0", len(got))
	}
}

func TestStableSortByRepo(t *testing.T) {
	prs := []github.PR{
		{Repo: "zzz/a", Title: "first"},
		{Repo: "aaa/b", Title: "second"},
		{Repo: "zzz/a", Title: "third"},
		{Repo: "aaa/b", Title: "fourth"},
	}
	stableSortByRepo(prs)
	if prs[0].Repo != "aaa/b" || prs[1].Repo != "aaa/b" {
		t.Errorf("first two should be aaa/b, got %q %q", prs[0].Repo, prs[1].Repo)
	}
	if prs[2].Repo != "zzz/a" || prs[3].Repo != "zzz/a" {
		t.Errorf("last two should be zzz/a, got %q %q", prs[2].Repo, prs[3].Repo)
	}
	// Stable: original order within same repo preserved.
	if prs[0].Title != "second" || prs[1].Title != "fourth" {
		t.Errorf("stability broken: aaa/b titles = %q, %q", prs[0].Title, prs[1].Title)
	}
}

func TestGroupedSort(t *testing.T) {
	prs := []github.PR{
		{Repo: "zzz/z", ReviewStatus: "APPROVED", CheckStatus: "SUCCESS"},
		{Repo: "aaa/a", CheckStatus: "FAILURE"},
		{Repo: "zzz/z", CheckStatus: "PENDING"},
		{Repo: "aaa/a", ReviewStatus: "APPROVED", CheckStatus: "SUCCESS"},
	}
	m := Model{
		selected: make(map[int]bool),
		sort:     sortByStatus,
	}
	m = m.SetPRs(prs)
	// Grouped: primary by repo, secondary by status.
	if m.filtered[0].Repo != "aaa/a" {
		t.Errorf("first repo = %q, want aaa/a", m.filtered[0].Repo)
	}
	if m.filtered[2].Repo != "zzz/z" {
		t.Errorf("third repo = %q, want zzz/z", m.filtered[2].Repo)
	}
	// Within aaa/a group: ready (approved+success) before failure.
	if m.filtered[0].CheckStatus != "SUCCESS" {
		t.Errorf("aaa/a first should be ready, got CheckStatus=%q", m.filtered[0].CheckStatus)
	}
}

func TestViewGrouped_RepoHeaders(t *testing.T) {
	m := Model{
		selected: make(map[int]bool),
		width:    120,
		height:   20,
	}
	m = m.SetPRs([]github.PR{
		{Repo: "aaa/a", Title: "pr1"},
		{Repo: "bbb/b", Title: "pr2"},
	})
	view := m.View()
	if !strings.Contains(view, "aaa/a") || !strings.Contains(view, "bbb/b") {
		t.Error("grouped view should contain repo headers")
	}
}
