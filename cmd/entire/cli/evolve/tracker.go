package evolve

import (
	"fmt"
	"time"
)

// Tracker manages the lifecycle of improvement suggestions.
type Tracker struct {
	Records map[string]*SuggestionRecord
}

// NewTracker creates a new Tracker with an empty record map.
func NewTracker() *Tracker {
	return &Tracker{Records: make(map[string]*SuggestionRecord)}
}

// AddSuggestion registers a new suggestion.
func (t *Tracker) AddSuggestion(rec SuggestionRecord) {
	t.Records[rec.ID] = &rec
}

// Get returns a suggestion by ID, or nil if not found.
func (t *Tracker) Get(id string) *SuggestionRecord {
	return t.Records[id]
}

// Accept marks a suggestion as accepted and records the resolution time.
func (t *Tracker) Accept(id string) error {
	rec, ok := t.Records[id]
	if !ok {
		return fmt.Errorf("suggestion %q not found", id)
	}
	now := time.Now()
	rec.Status = "accepted"
	rec.ResolvedAt = &now
	return nil
}

// Reject marks a suggestion as rejected and records the resolution time.
func (t *Tracker) Reject(id string) error {
	rec, ok := t.Records[id]
	if !ok {
		return fmt.Errorf("suggestion %q not found", id)
	}
	now := time.Now()
	rec.Status = "rejected"
	rec.ResolvedAt = &now
	return nil
}

// MeasureImpact sets the pre/post average scores for impact analysis.
// Returns the updated record, or nil if the ID is not found.
func (t *Tracker) MeasureImpact(id string, scoresBefore, scoresAfter []float64) *SuggestionRecord {
	rec, ok := t.Records[id]
	if !ok {
		return nil
	}
	rec.PreAvgScore = avgOrNil(scoresBefore)
	rec.PostAvgScore = avgOrNil(scoresAfter)
	return rec
}

// Pending returns all suggestions with "pending" status.
func (t *Tracker) Pending() []SuggestionRecord {
	var result []SuggestionRecord
	for _, rec := range t.Records {
		if rec.Status == "pending" {
			result = append(result, *rec)
		}
	}
	return result
}

// avgOrNil computes the average of a float64 slice.
// Returns nil if the slice is empty.
func avgOrNil(scores []float64) *float64 {
	if len(scores) == 0 {
		return nil
	}
	var sum float64
	for _, s := range scores {
		sum += s
	}
	avg := sum / float64(len(scores))
	return &avg
}
