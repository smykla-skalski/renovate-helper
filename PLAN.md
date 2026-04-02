# gh-renovate-tracker

## Context

Multiple Renovate PRs pile up across kumahq and related repos. No single view to see status, mergeability, missing reviews, or failing checks. Manual triaging via GitHub UI is slow. This tool provides a terminal dashboard to track and act on all Renovate PRs from one place.

**Delivery**: gh CLI extension, installed via `gh ext install <owner>/gh-renovate-tracker`

## Directory Structure

```
gh-renovate-tracker/
├── main.go                          # Entry point: flags, config load, client init, TUI launch
├── go.mod
├── internal/
│   ├── config/
│   │   └── config.go                # YAML config loading (~/.config/gh-renovate-tracker/config.yaml)
│   ├── github/
│   │   ├── client.go                # go-gh GQLClient wrapper, FetchPRs(), mutations
│   │   ├── queries.go               # GraphQL query/mutation constants
│   │   └── types.go                 # Domain types: PR, CheckRun, Review, etc.
│   └── tui/
│       ├── app.go                   # Root Bubble Tea model, view routing, message dispatch
│       ├── commands.go              # tea.Cmd wrappers for all async API calls
│       ├── keys.go                  # Keybinding definitions
│       ├── styles.go                # Lipgloss styles
│       ├── list/
│       │   └── model.go            # PR table view (main screen)
│       ├── detail/
│       │   └── model.go            # Single PR detail view
│       ├── filter/
│       │   └── model.go            # Filter/search overlay
│       └── help/
│           └── model.go            # Help overlay
├── .github/workflows/
│   └── release.yml                  # gh-extension-precompile for cross-platform binaries
└── .goreleaser.yml
```

## Config

`~/.config/gh-renovate-tracker/config.yaml`:

```yaml
orgs:
  - kumahq
repos:
  - Kong/kong-mesh          # individual repos outside tracked orgs
author: "renovate[bot]"     # default, configurable for dependabot etc.
merge_method: squash         # squash | merge | rebase
refresh_interval: 5m
```

Resolution order: CLI flags > env vars > config file > defaults.

## Core Types (`internal/github/types.go`)

```go
type PR struct {
    ID           string
    Number       int
    Repo         string        // "owner/repo"
    Title        string
    URL          string
    State        string        // OPEN, CLOSED, MERGED
    Mergeable    string        // MERGEABLE, CONFLICTING, UNKNOWN
    ReviewStatus string        // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
    CheckStatus  string        // SUCCESS, FAILURE, PENDING, ""
    Checks       []CheckRun
    Reviews      []Review
    Labels       []string
    CreatedAt    time.Time
    UpdatedAt    time.Time
    Additions    int
    Deletions    int
}

type CheckRun struct {
    Name       string
    Status     string    // COMPLETED, IN_PROGRESS, QUEUED
    Conclusion string    // SUCCESS, FAILURE, NEUTRAL, CANCELLED, TIMED_OUT
    SuiteID    string    // for rerun mutation
}

type Review struct {
    Author string
    State  string    // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
}
```

## GitHub API Strategy

**Single GraphQL request** fetches all data using search aliases:

```graphql
query {
  org0: search(query: "org:kumahq author:renovate[bot] is:pr is:open", type: ISSUE, first: 100) {
    ...prFields
  }
  repo0: search(query: "repo:Kong/kong-mesh author:renovate[bot] is:pr is:open", type: ISSUE, first: 100) {
    ...prFields
  }
}
fragment prFields on SearchResultItemConnection {
  nodes {
    ... on PullRequest {
      id number title url state mergeable
      repository { nameWithOwner }
      reviewDecision
      commits(last:1) { nodes { commit { statusCheckRollup { contexts(first:50) { nodes {
        ... on CheckRun { name status conclusion checkSuite { id } }
        ... on StatusContext { context state }
      }}}}}}
      reviews(last:10) { nodes { author { login } state } }
      labels(first:10) { nodes { name } }
      additions deletions createdAt updatedAt
    }
  }
}
```

**Mutations** (each a separate GraphQL call):
- `mergePullRequest(input: {pullRequestId, mergeMethod})`
- `addPullRequestReview(input: {pullRequestId, event: APPROVE})`
- `addLabelsToLabelable(input: {labelableId, labelIds})`
- `rerequestCheckSuite(input: {checkSuiteId, repositoryId})` 

**Rate budget**: ~15-25 points per refresh. At 5min interval = ~1800/hr (budget: 5000/hr).

## TUI Views

### 1. PR List (main)

```
 gh-renovate-tracker                                    ↻ 30s ago  42 PRs
─────────────────────────────────────────────────────────────────────────
 Repo                  PR Title                    Status  Checks  Age
 kumahq/kuma           fix(deps): update go to..   ✓ Ready  ✓ 12/12  2d
 kumahq/kuma           chore(deps): update helm..  ✗ Conflict ✓ 12/12  5d
 kumahq/kuma-website   fix(deps): update gatsby..  ◐ Review  ✗ 3/8   1d
 Kong/kong-mesh        chore(deps): update proto.. ✓ Ready  ◐ 10/12  3h
─────────────────────────────────────────────────────────────────────────
 j/k:navigate  m:merge  a:approve  l:label  r:rerun  /:filter  ?:help
```

**Status colors**:
- Green `✓ Ready` — mergeable + checks pass + approved
- Red `✗ Conflict` — merge conflicts
- Yellow `◐ Review` — awaiting review
- Yellow `◐ Checks` — checks in progress
- Red `✗ Checks` — checks failing

**Sorting**: by status (ready first), then age. Togglable via `s`.

### 2. PR Detail (enter)

Shows full check list, reviews, labels, diff stats, direct link. `esc` to go back.

### 3. Filter (/)

Text input filtering by repo name, PR title, or status.

## Keybindings

| Key | Action |
|-----|--------|
| `j/k` | Navigate up/down |
| `enter` | View PR detail |
| `esc` | Back to list |
| `m` | Merge selected PR (confirm prompt) |
| `a` | Approve selected PR |
| `l` | Add label (text input) |
| `r` | Rerun failed checks |
| `R` | Force refresh |
| `space` | Toggle multi-select |
| `M` | Merge all selected (confirm) |
| `A` | Approve all selected |
| `/` | Filter |
| `s` | Cycle sort mode |
| `g` | Group by repo toggle |
| `o` | Open PR in browser |
| `?` | Help overlay |
| `q` | Quit |

## Implementation Phases

### Phase 1: Skeleton + Config + API ✅
- `go mod init`, deps: `go-gh/v2`, `yaml.v3`
- Config loading with defaults
- GraphQL client, `FetchPRs()` returning `[]PR`
- CLI: `go run main.go` prints PRs to stdout as validation

### Phase 2: TUI List View ✅
- Add deps: `bubbletea`, `bubbles`, `lipgloss`
- Root model with list sub-model
- Table rendering with color-coded status
- j/k navigation, status bar, auto-refresh via `tea.Tick`

### Phase 3: Actions ✅
- ✅ Merge, approve, rerun checks mutations
- ✅ Status bar feedback ("Merged kumahq/kuma#1234")
- ✅ Auto-refresh after mutation
- ✅ Inline confirmation for destructive actions (merge, batch merge)
- ✅ Add label via REST API + text input overlay

### Phase 4: Detail + Multi-select + Batch ✅
- ✅ Detail view showing checks, reviews, labels
- ✅ Filter overlay with text input
- ✅ Help overlay
- ✅ Space to multi-select, SelectedPRs(), ClearSelected()
- ✅ M batch merge (with confirmation), A batch approve
- ✅ Group-by-repo rendering with repo headers

### Phase 5: Polish + Release ✅
- ✅ `o` to open in browser
- ✅ Sort toggles
- ✅ `.goreleaser.yml` + `release.yml` workflow
- ⬜ README with install instructions

## Dependencies (5 direct)

- `github.com/cli/go-gh/v2` — gh token, GraphQL client
- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/bubbles` — table, textinput, spinner
- `github.com/charmbracelet/lipgloss` — styling
- `gopkg.in/yaml.v3` — config parsing

## Verification

1. `gh ext install . && gh renovate-tracker` — local install test
2. Verify PRs match `gh search prs --author "renovate[bot]" --state open --owner kumahq`
3. Test merge on a throwaway PR
4. Test approve, label, rerun on real Renovate PRs
5. Cross-compile: `goreleaser build --snapshot`
