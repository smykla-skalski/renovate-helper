# gh-renovate-tracker

Terminal dashboard for tracking and managing Renovate PRs across GitHub organizations and repositories. Installed as a [`gh` CLI extension](https://cli.github.com/manual/gh_extension).

## Features

- **Unified view** of all Renovate PRs across multiple orgs and repos
- **Batch operations** — merge or approve multiple PRs at once
- **Rich status** — mergeability, check results, review state at a glance
- **Actions** — merge, approve, rerun checks, add labels, open in browser
- **Fix CI** — edit branches in worktrees to fix failing checks
- **Filtering & sorting** — search by repo/title/status, sort by status/age/repo
- **Copy review links** — aggregate and copy PR URLs to clipboard
- **Auto-refresh** — configurable polling interval
- **Print mode** — non-interactive stdout output for scripting

## Install

Requires [GitHub CLI](https://cli.github.com/) (`gh`) with `gh auth login` completed.

```bash
gh ext install smykla-skalski/gh-renovate-helper
```

## Usage

```bash
# Use config file
gh renovate-tracker

# Override with flags
gh renovate-tracker --orgs kumahq,example-org --repos other-org/some-repo --refresh 3m

# Non-interactive print mode
gh renovate-tracker --orgs kumahq --print
```

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--orgs` | Comma-separated org names | — |
| `--repos` | Comma-separated `owner/repo` pairs | — |
| `--author` | PR author filter | `renovate[bot]` |
| `--merge-method` | `squash`, `merge`, or `rebase` | `squash` |
| `--refresh` | Auto-refresh interval | `5m` |
| `--print` | Print PRs to stdout and exit | `false` |

## Configuration

Config file: `~/.config/gh-renovate-tracker/config.yaml`

CLI flags take precedence over config file values.

```yaml
author: "renovate[bot]"
merge_method: "squash"
refresh_interval: 5m
orgs:
  - kumahq
  - example-org
repos:
  - other-org/some-repo
exclude_repos:
  - owner/repo-to-skip
```

## Keybindings

### Navigation

| Key | Action |
|-----|--------|
| `j` / `k` / `↑` / `↓` | Move up / down |
| `enter` | Open PR detail view |
| `esc` | Back to list |
| `q` / `ctrl+c` | Quit |

### Actions

| Key | Action |
|-----|--------|
| `m` | Merge selected PR |
| `M` | Merge all selected PRs |
| `a` | Approve selected PR |
| `A` | Approve all selected PRs |
| `r` | Rerun failed check suites |
| `l` | Add label |
| `o` | Open PR in browser |
| `f` | Fix CI (worktree-based editing) |
| `c` | Copy review links to clipboard |

### View

| Key | Action |
|-----|--------|
| `/` | Filter PRs |
| `s` | Cycle sort mode (status → age → repo) |
| `space` | Toggle multi-select |
| `R` | Force refresh |
| `?` | Help |

## Status Indicators

| Indicator | Meaning |
|-----------|---------|
| `✓ Ready` | Mergeable, checks pass, approved |
| `✗ Conflict` | Merge conflicts |
| `◐ Review` | Awaiting review |
| `◐ Checks` | Checks in progress |
| `✗ Checks` | Checks failing |
| `✗ Timed Out` | Checks timed out |

## Development

### Prerequisites

- [mise](https://mise.jdx.dev/) for tool management
- `gh auth login` completed

```bash
# Install tools
mise install

# Run
go run main.go --orgs kumahq

# Build
go build -o gh-renovate-tracker .

# Test
go test ./...

# Lint
golangci-lint run ./...

# Snapshot release
goreleaser build --snapshot --clean
```

### Logs

Debug logs written to `/tmp/renovate-helper.log`.

```bash
tail -f /tmp/renovate-helper.log
```

## License

MIT
