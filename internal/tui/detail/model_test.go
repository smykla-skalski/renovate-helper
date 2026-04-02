package detail

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

const clipInput = "hello"

// --- clip ---

func TestClip_Short(t *testing.T) {
	if got := clip(clipInput, 10); got != clipInput {
		t.Errorf("clip short = %q, want %q", got, clipInput)
	}
}

func TestClip_Exact(t *testing.T) {
	if got := clip(clipInput, 5); got != clipInput {
		t.Errorf("clip exact = %q, want %q", got, clipInput)
	}
}

func TestClip_Truncated(t *testing.T) {
	got := clip("hello world", 7)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("clip truncated should end with …, got %q", got)
	}
	runes := []rune(got)
	if len(runes) != 7 {
		t.Errorf("clip truncated len = %d, want 7", len(runes))
	}
}

func TestClip_ZeroWidth(t *testing.T) {
	if got := clip(clipInput, 0); got != clipInput {
		t.Errorf("clip zero width = %q, want original", got)
	}
}

// --- coloredState ---

func TestColoredState_Open(t *testing.T) {
	got := coloredState("OPEN")
	if !strings.Contains(got, "OPEN") || !strings.Contains(got, "●") {
		t.Errorf("coloredState OPEN = %q, missing ● or OPEN", got)
	}
}

func TestColoredState_Merged(t *testing.T) {
	got := coloredState("MERGED")
	if !strings.Contains(got, "MERGED") || !strings.Contains(got, "⎇") {
		t.Errorf("coloredState MERGED = %q, missing ⎇ or MERGED", got)
	}
}

func TestColoredState_Closed(t *testing.T) {
	got := coloredState("CLOSED")
	if !strings.Contains(got, "CLOSED") || !strings.Contains(got, "✕") {
		t.Errorf("coloredState CLOSED = %q, missing ✕ or CLOSED", got)
	}
}

// --- coloredMergeable ---

func TestColoredMergeable_Mergeable(t *testing.T) {
	got := coloredMergeable("MERGEABLE")
	if !strings.Contains(got, "✓") {
		t.Errorf("coloredMergeable MERGEABLE missing ✓: %q", got)
	}
}

func TestColoredMergeable_Conflicting(t *testing.T) {
	got := coloredMergeable("CONFLICTING")
	if !strings.Contains(got, "✗") {
		t.Errorf("coloredMergeable CONFLICTING missing ✗: %q", got)
	}
}

func TestColoredMergeable_Unknown(t *testing.T) {
	got := coloredMergeable("UNKNOWN")
	if !strings.Contains(got, "◌") {
		t.Errorf("coloredMergeable unknown missing ◌: %q", got)
	}
}

// --- coloredReview ---

func TestColoredReview_Approved(t *testing.T) {
	got := coloredReview("APPROVED")
	if !strings.Contains(got, "✓") {
		t.Errorf("coloredReview APPROVED missing ✓: %q", got)
	}
}

func TestColoredReview_ChangesRequested(t *testing.T) {
	got := coloredReview("CHANGES_REQUESTED")
	if !strings.Contains(got, "✗") {
		t.Errorf("coloredReview CHANGES_REQUESTED missing ✗: %q", got)
	}
}

func TestColoredReview_ReviewRequired(t *testing.T) {
	got := coloredReview("REVIEW_REQUIRED")
	if !strings.Contains(got, "⚠") {
		t.Errorf("coloredReview REVIEW_REQUIRED missing ⚠: %q", got)
	}
}

// --- coloredReviewState ---

func TestColoredReviewState_Approved(t *testing.T) {
	got := coloredReviewState("APPROVED")
	if !strings.Contains(got, "✓") || !strings.Contains(got, "APPROVED") {
		t.Errorf("coloredReviewState APPROVED = %q", got)
	}
}

func TestColoredReviewState_ChangesRequested(t *testing.T) {
	got := coloredReviewState("CHANGES_REQUESTED")
	if !strings.Contains(got, "✗") {
		t.Errorf("coloredReviewState CHANGES_REQUESTED = %q", got)
	}
}

func TestColoredReviewState_Commented(t *testing.T) {
	got := coloredReviewState("COMMENTED")
	if !strings.Contains(got, "◎") {
		t.Errorf("coloredReviewState COMMENTED = %q", got)
	}
}

func TestColoredReviewState_Dismissed(t *testing.T) {
	got := coloredReviewState("DISMISSED")
	if !strings.Contains(got, "◌") {
		t.Errorf("coloredReviewState DISMISSED = %q", got)
	}
}

// --- Model construction ---

func TestNew_InitialState(t *testing.T) {
	pr := github.PR{Title: "fix: something", Repo: "org/repo"}
	m := New(pr)
	if m.scroll != 0 {
		t.Errorf("initial scroll = %d, want 0", m.scroll)
	}
	if m.width != 0 || m.height != 0 {
		t.Errorf("initial size = %dx%d, want 0x0", m.width, m.height)
	}
}

func TestSetSize(t *testing.T) {
	m := New(github.PR{}).SetSize(120, 40)
	if m.width != 120 || m.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", m.width, m.height)
	}
}

// --- Update / scroll ---

func newModel() Model {
	return New(github.PR{
		Title: "chore(deps): bump something",
		URL:   "https://github.com/org/repo/pull/1",
		Repo:  "org/repo",
		State: "OPEN",
	}).SetSize(120, 40)
}

func TestUpdate_KeyDown_Scrolls(t *testing.T) {
	m := newModel()
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m2.scroll != 1 {
		t.Errorf("scroll after down = %d, want 1", m2.scroll)
	}
}

func TestUpdate_KeyUp_NoNegative(t *testing.T) {
	m := newModel()
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m2.scroll != 0 {
		t.Errorf("scroll at 0 after up = %d, want 0", m2.scroll)
	}
}

func TestUpdate_KeyUp_Decrements(t *testing.T) {
	m := newModel()
	m.scroll = 5
	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m2.scroll != 4 {
		t.Errorf("scroll after up = %d, want 4", m2.scroll)
	}
}

func TestUpdate_VimJ_Scrolls(t *testing.T) {
	m := newModel()
	m2, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	if m2.scroll != 1 {
		t.Errorf("scroll after j = %d, want 1", m2.scroll)
	}
}

func TestUpdate_VimK_NoNegative(t *testing.T) {
	m := newModel()
	m2, _ := m.Update(tea.KeyPressMsg{Text: "k"})
	if m2.scroll != 0 {
		t.Errorf("scroll at 0 after k = %d, want 0", m2.scroll)
	}
}

func TestUpdate_MouseWheelDown_Scrolls(t *testing.T) {
	m := newModel()
	m2, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m2.scroll != 3 {
		t.Errorf("scroll after wheel down = %d, want 3", m2.scroll)
	}
}

func TestUpdate_MouseWheelUp_NoNegative(t *testing.T) {
	m := newModel()
	m2, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m2.scroll != 0 {
		t.Errorf("scroll at 0 after wheel up = %d, want 0", m2.scroll)
	}
}

func TestUpdate_MouseWheelUp_Decrements(t *testing.T) {
	m := newModel()
	m.scroll = 5
	m2, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m2.scroll != 2 {
		t.Errorf("scroll after wheel up = %d, want 2", m2.scroll)
	}
}

// --- View content ---

func newDetailModel() Model {
	return New(github.PR{
		Title:        "chore(deps): bump cert-manager from 1.20.0 to 1.20.1",
		URL:          "https://github.com/org/repo/pull/42",
		Repo:         "org/repo",
		State:        "OPEN",
		Mergeable:    "MERGEABLE",
		ReviewStatus: "REVIEW_REQUIRED",
		Additions:    3,
		Deletions:    3,
		Labels:       []string{"dependencies"},
		Checks: []github.CheckRun{
			{Name: "ci/build", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Name: "ci/test", Status: "COMPLETED", Conclusion: "FAILURE"},
		},
		Reviews: []github.Review{
			{Author: "alice", State: "APPROVED"},
		},
	}).SetSize(120, 40)
}

func TestView_ContainsTitle(t *testing.T) {
	m := newDetailModel()
	view := m.View()
	if !strings.Contains(view, "cert-manager") {
		t.Error("view missing title content")
	}
}

func TestView_ContainsRepo(t *testing.T) {
	m := newDetailModel()
	if !strings.Contains(m.View(), "org/repo") {
		t.Error("view missing repo")
	}
}

func TestView_ContainsDiff(t *testing.T) {
	m := newDetailModel()
	view := m.View()
	if !strings.Contains(view, "+3") || !strings.Contains(view, "-3") {
		t.Error("view missing diff counts")
	}
}

func TestView_FailedChecksShown(t *testing.T) {
	m := newDetailModel()
	view := m.View()
	if !strings.Contains(view, "ci/test") {
		t.Error("view missing failed check name")
	}
}

func TestView_PassedChecksNotListed(t *testing.T) {
	m := New(github.PR{
		Checks: []github.CheckRun{
			{Name: "ci/build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		},
	}).SetSize(120, 40)
	view := m.View()
	if !strings.Contains(view, "all passed") {
		t.Error("view should show 'all passed'")
	}
	if strings.Contains(view, "ci/build") {
		t.Error("view should not list individual passed checks")
	}
}

func TestView_ReviewShown(t *testing.T) {
	m := newDetailModel()
	if !strings.Contains(m.View(), "alice") {
		t.Error("view missing reviewer")
	}
}

func TestView_LabelShown(t *testing.T) {
	m := newDetailModel()
	if !strings.Contains(m.View(), "dependencies") {
		t.Error("view missing label")
	}
}

func TestView_BoxFillsHeight(t *testing.T) {
	m := newDetailModel()
	view := m.View()
	lines := strings.Split(view, "\n")
	// box outer height = m.height = 40
	if len(lines) != 40 {
		t.Errorf("view line count = %d, want 40 (m.height)", len(lines))
	}
}
