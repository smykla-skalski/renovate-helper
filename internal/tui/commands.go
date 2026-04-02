package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/klaudiush/gh-renovate-tracker/internal/config"
	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

type (
	prsLoadedMsg  struct{ prs []github.PR }
	errMsg        struct{ err error }
	actionDoneMsg struct{ msg string }
)

func fetchPRsCmd(client *github.Client, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		prs, err := client.FetchPRs(cfg)
		if err != nil {
			return errMsg{err}
		}
		return prsLoadedMsg{prs}
	}
}

func mergePRCmd(client *github.Client, pr github.PR, method string) tea.Cmd {
	return func() tea.Msg {
		if err := client.MergePR(pr.ID, method); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Merged " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
	}
}

func approvePRCmd(client *github.Client, pr github.PR) tea.Cmd {
	return func() tea.Msg {
		if err := client.ApprovePR(pr.ID); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Approved " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
	}
}

func rerunChecksCmd(client *github.Client, pr github.PR) tea.Cmd {
	return func() tea.Msg {
		parts := strings.SplitN(pr.Repo, "/", 2)
		if len(parts) != 2 {
			return errMsg{err: fmt.Errorf("invalid repo: %s", pr.Repo)}
		}
		var suiteIDs []string
		for _, cr := range pr.Checks {
			if cr.Conclusion == "FAILURE" || cr.Conclusion == "TIMED_OUT" {
				if cr.SuiteID != "" {
					suiteIDs = append(suiteIDs, cr.SuiteID)
				}
			}
		}
		if err := client.RerunChecks(parts[0], parts[1], suiteIDs); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Rerun checks for " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
	}
}
