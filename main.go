package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/klaudiush/gh-renovate-tracker/internal/config"
	"github.com/klaudiush/gh-renovate-tracker/internal/github"
	"github.com/klaudiush/gh-renovate-tracker/internal/tui"
)

func main() {
	var (
		orgs            = flag.String("orgs", "", "comma-separated orgs to track")
		repos           = flag.String("repos", "", "comma-separated owner/repo pairs")
		author          = flag.String("author", "", "PR author (default: renovate[bot])")
		mergeMethod     = flag.String("merge-method", "", "merge|squash|rebase")
		refreshInterval = flag.Duration("refresh", 0, "refresh interval (e.g. 5m)")
		printOnly       = flag.Bool("print", false, "print PRs to stdout and exit")
	)
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if *orgs != "" {
		cfg.Orgs = strings.Split(*orgs, ",")
	}
	if *repos != "" {
		cfg.Repos = strings.Split(*repos, ",")
	}
	if *author != "" {
		cfg.Author = *author
	}
	if *mergeMethod != "" {
		cfg.MergeMethod = *mergeMethod
	}
	if *refreshInterval != 0 {
		cfg.RefreshInterval = *refreshInterval
	}

	client, err := github.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "github client error: %v\n", err)
		os.Exit(1)
	}

	if *printOnly {
		prs, err := client.FetchPRs(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch error: %v\n", err)
			os.Exit(1)
		}
		for _, pr := range prs {
			fmt.Printf("%s #%d %s [%s] [%s]\n",
				pr.Repo, pr.Number, pr.Title, pr.ReviewStatus, pr.CheckStatus)
		}
		return
	}

	p := tea.NewProgram(
		tui.New(client, cfg),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
		os.Exit(1)
	}
}
