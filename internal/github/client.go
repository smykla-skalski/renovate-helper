package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gogh "github.com/cli/go-gh/v2/pkg/api"

	"github.com/smykla-skalski/gh-renovate-helper/internal/config"
)

const (
	retryMaxAttempts = 3
	retryBaseDelay   = 10 * time.Second
	retryMaxDelay    = 60 * time.Second
)

const (
	statusFailure       = "FAILURE"
	statusSuccess       = "SUCCESS"
	statusPending       = "PENDING"
	statusApproved      = "APPROVED"
	statusChanges       = "CHANGES_REQUESTED"
	statusReviewReq     = "REVIEW_REQUIRED"
	checkCompleted      = "COMPLETED"
	conclusionTimedOut  = "TIMED_OUT"
	conclusionCancelled = "CANCELLED"
	stateError          = "ERROR"
)

type Client struct {
	gql  *gogh.GraphQLClient
	rest *gogh.RESTClient
}

func NewClient() (*Client, error) {
	gql, err := gogh.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	rest, err := gogh.DefaultRESTClient()
	if err != nil {
		return nil, err
	}
	return &Client{gql: gql, rest: rest}, nil
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *gogh.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode >= 500 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "rate limit")
}

func (c *Client) doWithRetry(query string, vars map[string]any, result any) error {
	for attempt := range retryMaxAttempts {
		err := c.gql.Do(query, vars, result)
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		if attempt == retryMaxAttempts-1 {
			return err
		}
		delay := min(retryBaseDelay<<uint(attempt), retryMaxDelay)
		slog.Warn("retryable error, retrying", "attempt", attempt+1, "delay", delay, "error", err)
		time.Sleep(delay)
	}
	return nil // unreachable
}

func (c *Client) FetchPRs(cfg *config.Config) ([]PR, error) {
	if len(cfg.Orgs) == 0 && len(cfg.Repos) == 0 {
		return nil, fmt.Errorf("no orgs or repos configured")
	}

	slog.Debug("fetching PRs", "orgs", cfg.Orgs, "repos", cfg.Repos, "author", cfg.Author)

	excluded := make(map[string]bool, len(cfg.ExcludeRepos))
	for _, r := range cfg.ExcludeRepos {
		excluded[r] = true
	}

	var scopes []string
	for _, org := range cfg.Orgs {
		scopes = append(scopes, "org:"+org)
	}
	for _, repo := range cfg.Repos {
		scopes = append(scopes, "repo:"+repo)
	}

	var prs []PR
	for _, scope := range scopes {
		query := buildSingleSearchQuery(scope, cfg.Author)
		var result map[string]searchResult
		if err := c.doWithRetry(query, nil, &result); err != nil {
			slog.Error("graphql fetch failed", "scope", scope, "error", err)
			return nil, err
		}
		res, ok := result["result"]
		if !ok {
			continue
		}
		for i := range res.Nodes {
			pr := convertNode(&res.Nodes[i])
			if excluded[pr.Repo] {
				continue
			}
			prs = append(prs, pr)
		}
	}
	slog.Info("fetched PRs", "count", len(prs))
	return prs, nil
}

func (c *Client) FetchRepoPRs(repo string, cfg *config.Config) ([]PR, error) {
	slog.Debug("fetching PRs for repo", "repo", repo)
	query := buildSingleSearchQuery("repo:"+repo, cfg.Author)

	var result map[string]searchResult
	if err := c.doWithRetry(query, nil, &result); err != nil {
		slog.Error("graphql repo fetch failed", "repo", repo, "error", err)
		return nil, err
	}

	res, ok := result["result"]
	if !ok {
		return nil, nil
	}
	prs := make([]PR, 0, len(res.Nodes))
	for i := range res.Nodes {
		prs = append(prs, convertNode(&res.Nodes[i]))
	}
	slog.Info("fetched repo PRs", "repo", repo, "count", len(prs))
	return prs, nil
}

func buildSingleSearchQuery(scope, author string) string {
	q := fmt.Sprintf("%s author:%s is:pr is:open", scope, author)
	return fmt.Sprintf("query {\n  result: search(query: %q, type: ISSUE, first: 100) { ...prFields }\n}\n%s", q, prFragment)
}

func (c *Client) MergePR(prID, mergeMethod string) error {
	method := strings.ToUpper(mergeMethod)
	slog.Info("merging PR", "id", prID, "method", method)
	var result map[string]any
	vars := map[string]any{"id": prID, "method": method}
	if err := c.doWithRetry(mergeMutation, vars, &result); err != nil {
		slog.Error("merge failed", "id", prID, "error", err)
		return err
	}
	slog.Info("merge complete", "id", prID)
	return nil
}

func (c *Client) ApprovePR(prID string) error {
	slog.Info("approving PR", "id", prID)
	var result map[string]any
	vars := map[string]any{"id": prID}
	if err := c.doWithRetry(approveMutation, vars, &result); err != nil {
		slog.Error("approve failed", "id", prID, "error", err)
		return err
	}
	return nil
}

func (c *Client) RerunChecks(repoOwner, repoName string, suiteIDs []string) error {
	slog.Info("rerunning checks", "repo", repoOwner+"/"+repoName, "suites", len(suiteIDs))
	repoID, err := c.fetchRepoID(repoOwner, repoName)
	if err != nil {
		slog.Error("fetch repo ID failed", "repo", repoOwner+"/"+repoName, "error", err)
		return err
	}
	for _, sid := range suiteIDs {
		var result map[string]any
		vars := map[string]any{"checkSuiteId": sid, "repositoryId": repoID}
		if err := c.doWithRetry(rerequestCheckSuiteMutation, vars, &result); err != nil {
			slog.Error("rerun check suite failed", "suite", sid, "error", err)
			return err
		}
	}
	return nil
}

func (c *Client) AddLabel(owner, repo string, number int, label string) error {
	path := fmt.Sprintf("repos/%s/%s/issues/%d/labels", owner, repo, number)
	payload := struct {
		Labels []string `json:"labels"`
	}{Labels: []string{label}}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.rest.Post(path, bytes.NewReader(body), nil)
}

func (c *Client) fetchRepoID(owner, name string) (string, error) {
	var result struct {
		Repository struct{ ID string }
	}
	vars := map[string]any{"owner": owner, "name": name}
	if err := c.doWithRetry(repoIDQuery, vars, &result); err != nil {
		return "", err
	}
	return result.Repository.ID, nil
}

type searchResult struct {
	Nodes []prNode
}

type prNode struct {
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ID             string
	Repository     struct{ NameWithOwner string }
	Title          string
	URL            string
	State          string
	Mergeable      string
	ReviewDecision string
	Labels         struct{ Nodes []struct{ Name string } }
	Reviews        struct {
		Nodes []struct {
			Author struct{ Login string }
			State  string
		}
	}
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct{ Nodes []checkContext }
				}
			}
		}
	}
	Number    int
	Additions int
	Deletions int
}

type checkContext struct {
	Name       string
	Status     string
	Conclusion string
	CheckSuite struct{ ID string }
	Context    string
	State      string
}

func convertNode(n *prNode) PR {
	pr := PR{
		ID:        n.ID,
		Number:    n.Number,
		Title:     n.Title,
		URL:       n.URL,
		State:     n.State,
		Mergeable: n.Mergeable,
		Repo:      n.Repository.NameWithOwner,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
		Additions: n.Additions,
		Deletions: n.Deletions,
	}

	switch n.ReviewDecision {
	case statusApproved:
		pr.ReviewStatus = statusApproved
	case statusChanges:
		pr.ReviewStatus = statusChanges
	case statusReviewReq:
		pr.ReviewStatus = statusReviewReq
	}

	for i := range n.Reviews.Nodes {
		pr.Reviews = append(pr.Reviews, Review{
			Author: n.Reviews.Nodes[i].Author.Login,
			State:  n.Reviews.Nodes[i].State,
		})
	}

	for i := range n.Labels.Nodes {
		pr.Labels = append(pr.Labels, n.Labels.Nodes[i].Name)
	}

	if len(n.Commits.Nodes) > 0 {
		commit := n.Commits.Nodes[0].Commit
		if commit.StatusCheckRollup != nil {
			pending, failed, total := 0, 0, 0
			for i := range commit.StatusCheckRollup.Contexts.Nodes {
				ctx := &commit.StatusCheckRollup.Contexts.Nodes[i]
				if ctx.Name != "" {
					total++
					cr := CheckRun{
						Name:       ctx.Name,
						Status:     ctx.Status,
						Conclusion: ctx.Conclusion,
						SuiteID:    ctx.CheckSuite.ID,
					}
					pr.Checks = append(pr.Checks, cr)
					switch {
					case ctx.Status != checkCompleted:
						pending++
					case ctx.Conclusion == statusFailure || ctx.Conclusion == conclusionTimedOut || ctx.Conclusion == conclusionCancelled:
						failed++
					}
				} else if ctx.Context != "" {
					if ctx.Context == "renovate/stability-days" {
						if ctx.State != statusSuccess {
							pr.StabilityDays = true
						}
						continue
					}
					total++
					cr := CheckRun{Name: ctx.Context, Status: checkCompleted, Conclusion: ctx.State}
					pr.Checks = append(pr.Checks, cr)
					if ctx.State == statusFailure || ctx.State == stateError {
						failed++
					}
				}
			}
			switch {
			case total == 0:
				pr.CheckStatus = ""
			case failed > 0:
				pr.CheckStatus = statusFailure
			case pending > 0:
				pr.CheckStatus = statusPending
			default:
				pr.CheckStatus = statusSuccess
			}
		}
	}

	return pr
}
