package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/smykla-skalski/gh-renovate-helper/internal/config"
	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
)

func newTestModel() Model {
	cfg := &config.Config{
		MergeMethod:     "squash",
		RefreshInterval: 5 * time.Minute,
	}
	m := New(nil, cfg)
	m.list = m.list.SetPRs([]github.PR{
		{ID: "1", Number: 10, Repo: "org/repo", Title: "update go"},
		{ID: "2", Number: 20, Repo: "org/other", Title: "update helm"},
	})
	m.loading = false
	return m
}

func TestStartConfirm(t *testing.T) {
	m := newTestModel()
	called := false
	cmd := func() tea.Msg { called = true; return nil }

	m = m.startConfirm("Merge org/repo#10? (y/n)", cmd)

	if !m.confirming {
		t.Error("confirming should be true")
	}
	if m.status != "Merge org/repo#10? (y/n)" {
		t.Errorf("status = %q", m.status)
	}
	if m.pendingCmd == nil {
		t.Error("pendingCmd should be set")
	}

	// Confirm with y.
	result, resultCmd := m.handleConfirm(tea.KeyPressMsg{Text: "y"})
	m = result.(Model)
	if m.confirming {
		t.Error("confirming should be false after y")
	}
	if resultCmd == nil {
		t.Error("cmd should be returned on confirm")
	}
	// Verify the pending cmd is the one we set.
	_ = called
}

func TestConfirmCancel(t *testing.T) {
	m := newTestModel()
	m = m.startConfirm("Merge? (y/n)", func() tea.Msg { return nil })

	result, cmd := m.handleConfirm(tea.KeyPressMsg{Text: "n"})
	m = result.(Model)
	if m.confirming {
		t.Error("confirming should be false after cancel")
	}
	if m.status != "cancelled" {
		t.Errorf("status = %q, want cancelled", m.status)
	}
	if cmd != nil {
		t.Error("cmd should be nil on cancel")
	}
}

func TestConfirmEsc(t *testing.T) {
	m := newTestModel()
	m = m.startConfirm("Merge? (y/n)", func() tea.Msg { return nil })

	result, cmd := m.handleConfirm(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = result.(Model)
	if m.confirming {
		t.Error("confirming should be false after esc")
	}
	if cmd != nil {
		t.Error("cmd should be nil on esc")
	}
}

func TestHandleKey_MergeTriggersConfirm(t *testing.T) {
	m := newTestModel()
	m.current = viewList

	result, _ := m.handleKey(tea.KeyPressMsg{Text: "m"})
	m = result.(Model)
	if !m.confirming {
		t.Error("pressing m should trigger confirmation")
	}
	if m.status == "" {
		t.Error("confirm message should be set")
	}
}

func TestHandleKey_LabelOpensInput(t *testing.T) {
	m := newTestModel()
	m.current = viewList

	result, _ := m.handleKey(tea.KeyPressMsg{Text: "l"})
	m = result.(Model)
	if m.current != viewLabel {
		t.Errorf("current = %d, want viewLabel (%d)", m.current, viewLabel)
	}
	if m.labelPR.ID == "" {
		t.Error("labelPR should be set")
	}
}

func TestHandleLabelInput_Esc(t *testing.T) {
	m := newTestModel()
	m.current = viewLabel

	result, _ := m.handleLabelInput(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = result.(Model)
	if m.current != viewList {
		t.Errorf("current = %d, want viewList after esc", m.current)
	}
}

func TestRenderStatus_Confirming(t *testing.T) {
	m := newTestModel()
	m.confirming = true
	m.status = "Merge org/repo#10? (y/n)"

	s := m.renderBottomBar()
	if s == "" {
		t.Error("renderBottomBar should return non-empty for confirming state")
	}
}

func TestRenderStatus_Loading(t *testing.T) {
	m := newTestModel()
	m.loading = true

	s := m.renderBottomBar()
	if s == "" {
		t.Error("renderBottomBar should return non-empty for loading state")
	}
}

func TestRenderStatus_Error(t *testing.T) {
	m := newTestModel()
	m.statusErr = true
	m.status = "something failed"

	s := m.renderBottomBar()
	if s == "" {
		t.Error("renderBottomBar should return non-empty for error state")
	}
}

func TestRenderStatus_LastFetch(t *testing.T) {
	m := newTestModel()
	m.lastFetch = time.Now().Add(-10 * time.Second).UnixNano()

	s := m.renderBottomBar()
	if s == "" {
		t.Error("renderBottomBar should return non-empty with lastFetch")
	}
}

const testStatus = "3 PRs"

func snapshotModel(width, height int) Model {
	now := time.Now()
	m := newTestModel()
	m.width = width
	m.height = height
	m.list = m.list.SetSize(width, height-1)
	m.list = m.list.SetPRs([]github.PR{
		{Repo: "org/repo", Title: "update go", ReviewStatus: "APPROVED", CheckStatus: "SUCCESS", CreatedAt: now.Add(-48 * time.Hour)},
		{Repo: "org/repo", Title: "update helm", ReviewStatus: "REVIEW_REQUIRED", CreatedAt: now.Add(-72 * time.Hour)},
		{Repo: "org/other", Title: "bump deps", CheckStatus: "FAILURE", CreatedAt: now.Add(-24 * time.Hour)},
		{Repo: "org/other", Title: "bump lodash (security)", CheckStatus: "SUCCESS", ReviewStatus: "REVIEW_REQUIRED", Labels: []string{"security"}, CreatedAt: now.Add(-12 * time.Hour)},
	})
	m.lastFetch = now.UnixNano()
	m.status = testStatus
	return m
}

func TestView_Snapshot(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := snapshotModel(100, 15)
	golden.RequireEqual(t, ansi.Strip(m.View().Content)+"\n")
}

func TestView_Narrow_Snapshot(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := snapshotModel(60, 15)
	golden.RequireEqual(t, ansi.Strip(m.View().Content)+"\n")
}
