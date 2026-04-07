package github

import "time"

type PR struct {
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	URL           string     `json:"url"`
	State         string     `json:"state"`
	CheckStatus   string     `json:"check_status"`
	ID            string     `json:"id"`
	Repo          string     `json:"repo"`
	Title         string     `json:"title"`
	ReviewStatus  string     `json:"review_status"`
	Mergeable     string     `json:"mergeable"`
	Labels        []string   `json:"labels"`
	Checks        []CheckRun `json:"checks"`
	Reviews       []Review   `json:"reviews"`
	Number        int        `json:"number"`
	Additions     int        `json:"additions"`
	Deletions     int        `json:"deletions"`
	StabilityDays bool       `json:"stability_days"`
}

type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`     // COMPLETED, IN_PROGRESS, QUEUED
	Conclusion string `json:"conclusion"` // SUCCESS, FAILURE, NEUTRAL, CANCELLED, TIMED_OUT
	SuiteID    string `json:"suite_id"`   // for rerun mutation
}

type Review struct {
	Author string `json:"author"`
	State  string `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
}
