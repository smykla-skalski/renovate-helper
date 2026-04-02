# gh-renovate-tracker

Terminal dashboard for tracking and acting on Renovate PRs across GitHub orgs/repos. Installed as a `gh` CLI extension.

## Project Structure

```
gh-renovate-tracker/
├── main.go                    # Entry: flags, config, client init, TUI launch
├── internal/
│   ├── config/config.go       # YAML config (~/.config/gh-renovate-tracker/config.yaml)
│   ├── github/
│   │   ├── client.go          # go-gh GQLClient wrapper, FetchPRs(), mutations
│   │   ├── queries.go         # GraphQL query/mutation string constants
│   │   └── types.go           # Domain types: PR, CheckRun, Review
│   └── tui/
│       ├── app.go             # Root Bubble Tea model, view routing
│       ├── commands.go        # tea.Cmd wrappers for async API calls
│       ├── keys.go            # Keybinding definitions
│       ├── browser.go         # Open-in-browser helper
│       ├── list/model.go      # PR table view (main screen)
│       ├── detail/model.go    # Single PR detail view
│       ├── filter/model.go    # Filter/search overlay
│       └── help/model.go      # Help overlay
├── .golangci.yml              # Linter config (30+ linters)
├── .goreleaser.yml            # Cross-platform release builds
├── mise.toml                  # Tool versions (go, goreleaser)
└── PLAN.md                    # Architecture doc with full spec
```

## Tech Stack

- **Language:** Go 1.25+
- **TUI:** Bubble Tea v2 + Bubbles v2 + Lipgloss v2
- **GitHub API:** go-gh/v2 (GraphQL via gh CLI auth)
- **Config:** gopkg.in/yaml.v3
- **Linting:** golangci-lint v2 (gofumpt formatting)
- **Release:** goreleaser
- **Tools:** mise

## Architecture

### Bubble Tea Model Hierarchy

```
tui.Model (app.go)            # Root model, routes messages
├── list.Model (list/)        # PR table, main screen
├── detail.Model (detail/)    # Single PR detail
├── filter.Model (filter/)    # Search overlay
└── help.Model (help/)        # Help overlay
```

- Root model in `app.go` owns the GitHub client and config
- Sub-models receive data via messages, return tea.Cmd for async ops
- All API calls go through `commands.go` as tea.Cmd functions
- View routing: root model delegates Update/View to active sub-model

### GitHub API

- Single GraphQL query fetches all PRs using search aliases (one per org/repo)
- Mutations are separate calls: merge, approve, label, rerun checks
- Auth inherited from `gh` CLI — requires `gh auth login` first
- Rate budget: ~15-25 points per refresh at 5min interval

### Config Resolution

CLI flags > config file (`~/.config/gh-renovate-tracker/config.yaml`) > defaults

## Testing

- Unit tests alongside source files (`*_test.go`)
- Mock GitHub responses for client tests
- Run: `go test ./...`

## Quality Gates

Before committing:

```bash
# Lint (includes gofumpt formatting)
golangci-lint run ./...

# Test
go test ./...

# Build
go build ./...
```

All three must pass.

## Common Commands

```bash
# Run in dev
go run main.go --orgs kumahq

# Run with print mode (no TUI, stdout only)
go run main.go --orgs kumahq --print

# Build binary
go build -o gh-renovate-tracker .

# Install as gh extension (local)
gh ext install .

# Run as gh extension
gh renovate-tracker

# Snapshot release build
goreleaser build --snapshot --clean

# View logs
tail -f /tmp/renovate-helper.log
```

## Anti-Patterns

- Do not add lint suppression directives — fix the lint issue or restructure code
- Do not call GitHub API directly — use `commands.go` tea.Cmd wrappers for all async operations
- Do not modify sub-model state from root model — send messages instead
- Do not ignore struct field alignment — govet fieldalignment is enabled
- Do not hardcode org/repo names — use config
- Errors go to stderr via `fmt.Fprintf(os.Stderr, ...)`, never stdout
- Log file at `/tmp/renovate-helper.log` — use `slog` for debug logging, not fmt.Print
