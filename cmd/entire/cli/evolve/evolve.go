// Package evolve implements the evolution loop that tracks sessions since the
// last improvement run and triggers suggestions after a configurable threshold.
package evolve

import "time"

// State tracks the evolution loop's progress.
// Stored in SQLite and mirrored to insights/evolution.json on the checkpoint branch.
type State struct {
	LastRunAt            time.Time `json:"last_run_at"`
	SessionsSinceLastRun int       `json:"sessions_since_last_run"`
	TotalRuns            int       `json:"total_runs"`
	SuggestionsGenerated int       `json:"suggestions_generated"`
	SuggestionsAccepted  int       `json:"suggestions_accepted"`
}

// SuggestionRecord tracks a suggestion through its lifecycle.
type SuggestionRecord struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	FileType     string     `json:"file_type"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"` // "pending", "accepted", "rejected"
	CreatedAt    time.Time  `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	PreAvgScore  *float64   `json:"pre_avg_score,omitempty"`
	PostAvgScore *float64   `json:"post_avg_score,omitempty"`
}
