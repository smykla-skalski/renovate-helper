package github

import "time"

type PR struct {
	CreatedAt     time.Time
	UpdatedAt     time.Time
	URL           string
	State         string
	CheckStatus   string
	ID            string
	Repo          string
	Title         string
	ReviewStatus  string
	Mergeable     string
	Labels        []string
	Checks        []CheckRun
	Reviews       []Review
	Number        int
	Additions     int
	Deletions     int
	StabilityDays bool
}

type CheckRun struct {
	Name       string
	Status     string // COMPLETED, IN_PROGRESS, QUEUED
	Conclusion string // SUCCESS, FAILURE, NEUTRAL, CANCELLED, TIMED_OUT
	SuiteID    string // for rerun mutation
}

type Review struct {
	Author string
	State  string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
}
