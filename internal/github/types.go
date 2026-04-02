package github

import "time"

type PR struct {
	ID           string
	Number       int
	Repo         string // "owner/repo"
	Title        string
	URL          string
	State        string // OPEN, CLOSED, MERGED
	Mergeable    string // MERGEABLE, CONFLICTING, UNKNOWN
	ReviewStatus string // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	CheckStatus  string // SUCCESS, FAILURE, PENDING, ""
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
	Status     string // COMPLETED, IN_PROGRESS, QUEUED
	Conclusion string // SUCCESS, FAILURE, NEUTRAL, CANCELLED, TIMED_OUT
	SuiteID    string // for rerun mutation
}

type Review struct {
	Author string
	State  string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
}
