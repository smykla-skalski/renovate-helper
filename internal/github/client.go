package github

import (
	"fmt"
	"strings"
	"time"

	gogh "github.com/cli/go-gh/v2/pkg/api"

	"github.com/klaudiush/gh-renovate-tracker/internal/config"
)

type Client struct {
	gql *gogh.GraphQLClient
}

func NewClient() (*Client, error) {
	gql, err := gogh.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	return &Client{gql: gql}, nil
}

// FetchPRs fetches all Renovate PRs for the configured orgs and repos.
func (c *Client) FetchPRs(cfg *config.Config) ([]PR, error) {
	if len(cfg.Orgs) == 0 && len(cfg.Repos) == 0 {
		return nil, fmt.Errorf("no orgs or repos configured")
	}

	query, aliases := buildSearchQuery(cfg)
	var result map[string]searchResult
	if err := c.gql.Do(query, nil, &result); err != nil {
		return nil, err
	}

	var prs []PR
	for _, alias := range aliases {
		res, ok := result[alias]
		if !ok {
			continue
		}
		for _, node := range res.Nodes {
			prs = append(prs, convertNode(node))
		}
	}
	return prs, nil
}

func buildSearchQuery(cfg *config.Config) (string, []string) {
	var sb strings.Builder
	sb.WriteString("query {\n")
	var aliases []string

	for i, org := range cfg.Orgs {
		alias := fmt.Sprintf("org%d", i)
		aliases = append(aliases, alias)
		q := fmt.Sprintf("org:%s author:%s is:pr is:open", org, cfg.Author)
		sb.WriteString(fmt.Sprintf("  %s: search(query: %q, type: ISSUE, first: 100) { ...prFields }\n", alias, q))
	}
	for i, repo := range cfg.Repos {
		alias := fmt.Sprintf("repo%d", i)
		aliases = append(aliases, alias)
		q := fmt.Sprintf("repo:%s author:%s is:pr is:open", repo, cfg.Author)
		sb.WriteString(fmt.Sprintf("  %s: search(query: %q, type: ISSUE, first: 100) { ...prFields }\n", alias, q))
	}

	sb.WriteString("}\n")
	sb.WriteString(prFragment)
	return sb.String(), aliases
}

// MergePR merges a PR with the configured merge method.
func (c *Client) MergePR(prID, mergeMethod string) error {
	method := strings.ToUpper(mergeMethod)
	var result map[string]interface{}
	vars := map[string]interface{}{"id": prID, "method": method}
	return c.gql.Do(mergeMutation, vars, &result)
}

// ApprovePR approves a PR.
func (c *Client) ApprovePR(prID string) error {
	var result map[string]interface{}
	vars := map[string]interface{}{"id": prID}
	return c.gql.Do(approveMutation, vars, &result)
}

// RerunChecks rerequests all failed check suites for a PR.
func (c *Client) RerunChecks(repoOwner, repoName string, suiteIDs []string) error {
	repoID, err := c.fetchRepoID(repoOwner, repoName)
	if err != nil {
		return err
	}
	for _, sid := range suiteIDs {
		var result map[string]interface{}
		vars := map[string]interface{}{"checkSuiteId": sid, "repositoryId": repoID}
		if err := c.gql.Do(rerequestCheckSuiteMutation, vars, &result); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) fetchRepoID(owner, name string) (string, error) {
	var result struct {
		Repository struct{ ID string }
	}
	vars := map[string]interface{}{"owner": owner, "name": name}
	if err := c.gql.Do(repoIDQuery, vars, &result); err != nil {
		return "", err
	}
	return result.Repository.ID, nil
}

type searchResult struct {
	Nodes []prNode
}

type prNode struct {
	ID             string
	Number         int
	Title          string
	URL            string
	State          string
	Mergeable      string
	Repository     struct{ NameWithOwner string }
	ReviewDecision string
	Commits        struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []checkContext
					}
				}
			}
		}
	}
	Reviews struct {
		Nodes []struct {
			Author struct{ Login string }
			State  string
		}
	}
	Labels struct {
		Nodes []struct{ Name string }
	}
	Additions int
	Deletions int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type checkContext struct {
	Name       string
	Status     string
	Conclusion string
	CheckSuite struct{ ID string }
	Context    string
	State      string
}

func convertNode(n prNode) PR {
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
	case "APPROVED":
		pr.ReviewStatus = "APPROVED"
	case "CHANGES_REQUESTED":
		pr.ReviewStatus = "CHANGES_REQUESTED"
	case "REVIEW_REQUIRED":
		pr.ReviewStatus = "REVIEW_REQUIRED"
	}

	for _, r := range n.Reviews.Nodes {
		pr.Reviews = append(pr.Reviews, Review{
			Author: r.Author.Login,
			State:  r.State,
		})
	}

	for _, l := range n.Labels.Nodes {
		pr.Labels = append(pr.Labels, l.Name)
	}

	if len(n.Commits.Nodes) > 0 {
		commit := n.Commits.Nodes[0].Commit
		if commit.StatusCheckRollup != nil {
			pending, failed, total := 0, 0, 0
			for _, ctx := range commit.StatusCheckRollup.Contexts.Nodes {
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
					case ctx.Status != "COMPLETED":
						pending++
					case ctx.Conclusion == "FAILURE" || ctx.Conclusion == "TIMED_OUT" || ctx.Conclusion == "CANCELLED":
						failed++
					}
				} else if ctx.Context != "" {
					total++
					cr := CheckRun{Name: ctx.Context, Status: "COMPLETED", Conclusion: ctx.State}
					pr.Checks = append(pr.Checks, cr)
					if ctx.State == "FAILURE" || ctx.State == "ERROR" {
						failed++
					}
				}
			}
			switch {
			case total == 0:
				pr.CheckStatus = ""
			case failed > 0:
				pr.CheckStatus = "FAILURE"
			case pending > 0:
				pr.CheckStatus = "PENDING"
			default:
				pr.CheckStatus = "SUCCESS"
			}
		}
	}

	return pr
}
