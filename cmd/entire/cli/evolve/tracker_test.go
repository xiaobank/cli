package evolve_test

import (
	"math"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/evolve"
)

func TestTracker_AddAndGet(t *testing.T) {
	t.Parallel()

	tr := evolve.NewTracker()
	rec := evolve.SuggestionRecord{
		ID:        "rec-1",
		Title:     "Add lint instructions",
		FileType:  "CLAUDE.md",
		Priority:  "high",
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	tr.AddSuggestion(rec)

	got := tr.Get("rec-1")
	if got == nil {
		t.Fatal("expected to find record by ID, got nil")
	}
	if got.Title != "Add lint instructions" {
		t.Errorf("expected Title=%q, got %q", "Add lint instructions", got.Title)
	}
}

func TestTracker_Accept(t *testing.T) {
	t.Parallel()

	tr := evolve.NewTracker()
	tr.AddSuggestion(evolve.SuggestionRecord{
		ID:        "rec-2",
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	before := time.Now()
	if err := tr.Accept("rec-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now()

	got := tr.Get("rec-2")
	if got.Status != "accepted" {
		t.Errorf("expected Status=%q, got %q", "accepted", got.Status)
	}
	if got.ResolvedAt == nil {
		t.Fatal("expected ResolvedAt to be set")
	}
	if got.ResolvedAt.Before(before) || got.ResolvedAt.After(after) {
		t.Errorf("expected ResolvedAt to be approximately now, got %v", got.ResolvedAt)
	}
}

func TestTracker_Reject(t *testing.T) {
	t.Parallel()

	tr := evolve.NewTracker()
	tr.AddSuggestion(evolve.SuggestionRecord{
		ID:        "rec-3",
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	before := time.Now()
	if err := tr.Reject("rec-3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now()

	got := tr.Get("rec-3")
	if got.Status != "rejected" {
		t.Errorf("expected Status=%q, got %q", "rejected", got.Status)
	}
	if got.ResolvedAt == nil {
		t.Fatal("expected ResolvedAt to be set")
	}
	if got.ResolvedAt.Before(before) || got.ResolvedAt.After(after) {
		t.Errorf("expected ResolvedAt to be approximately now, got %v", got.ResolvedAt)
	}
}

func TestTracker_AcceptNotFound(t *testing.T) {
	t.Parallel()

	tr := evolve.NewTracker()

	if err := tr.Accept("nonexistent"); err == nil {
		t.Error("expected error for unknown ID, got nil")
	}
}

func TestTracker_RejectNotFound(t *testing.T) {
	t.Parallel()

	tr := evolve.NewTracker()

	if err := tr.Reject("nonexistent"); err == nil {
		t.Error("expected error for unknown ID, got nil")
	}
}

func TestTracker_MeasureImpact(t *testing.T) {
	t.Parallel()

	tr := evolve.NewTracker()
	tr.AddSuggestion(evolve.SuggestionRecord{
		ID:        "rec-4",
		Status:    "accepted",
		CreatedAt: time.Now(),
	})

	scoresBefore := []float64{0.6, 0.8, 0.7}
	scoresAfter := []float64{0.9, 0.85, 0.95}
	got := tr.MeasureImpact("rec-4", scoresBefore, scoresAfter)

	if got == nil {
		t.Fatal("expected non-nil SuggestionRecord")
	}
	if got.PreAvgScore == nil {
		t.Fatal("expected PreAvgScore to be set")
	}
	expectedPre := (0.6 + 0.8 + 0.7) / 3
	if math.Abs(*got.PreAvgScore-expectedPre) > 1e-9 {
		t.Errorf("expected PreAvgScore=%f, got %f", expectedPre, *got.PreAvgScore)
	}
	if got.PostAvgScore == nil {
		t.Fatal("expected PostAvgScore to be set")
	}
	expectedPost := (0.9 + 0.85 + 0.95) / 3
	if math.Abs(*got.PostAvgScore-expectedPost) > 1e-9 {
		t.Errorf("expected PostAvgScore=%f, got %f", expectedPost, *got.PostAvgScore)
	}
}

func TestTracker_MeasureImpact_EmptySlices(t *testing.T) {
	t.Parallel()

	tr := evolve.NewTracker()
	tr.AddSuggestion(evolve.SuggestionRecord{
		ID:        "rec-5",
		Status:    "accepted",
		CreatedAt: time.Now(),
	})

	got := tr.MeasureImpact("rec-5", nil, nil)
	if got == nil {
		t.Fatal("expected non-nil SuggestionRecord")
	}
	// Empty slices should result in nil scores (no data to average)
	if got.PreAvgScore != nil {
		t.Errorf("expected PreAvgScore=nil for empty slice, got %f", *got.PreAvgScore)
	}
	if got.PostAvgScore != nil {
		t.Errorf("expected PostAvgScore=nil for empty slice, got %f", *got.PostAvgScore)
	}
}

func TestTracker_Pending(t *testing.T) {
	t.Parallel()

	tr := evolve.NewTracker()
	tr.AddSuggestion(evolve.SuggestionRecord{ID: "p1", Status: "pending", CreatedAt: time.Now()})
	tr.AddSuggestion(evolve.SuggestionRecord{ID: "p2", Status: "pending", CreatedAt: time.Now()})
	tr.AddSuggestion(evolve.SuggestionRecord{ID: "a1", Status: "accepted", CreatedAt: time.Now()})
	tr.AddSuggestion(evolve.SuggestionRecord{ID: "r1", Status: "rejected", CreatedAt: time.Now()})

	pending := tr.Pending()

	if len(pending) != 2 {
		t.Errorf("expected 2 pending records, got %d", len(pending))
	}
	for _, rec := range pending {
		if rec.Status != "pending" {
			t.Errorf("expected Status=pending, got %q", rec.Status)
		}
	}
}
