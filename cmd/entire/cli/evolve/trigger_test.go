package evolve_test

import (
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/evolve"
	"github.com/entireio/cli/cmd/entire/cli/settings"
)

func TestShouldTrigger_ThresholdMet(t *testing.T) {
	t.Parallel()

	config := settings.EvolveSettings{Enabled: true, SessionThreshold: 5}
	state := evolve.State{SessionsSinceLastRun: 5}

	if !evolve.ShouldTrigger(config, state) {
		t.Error("expected ShouldTrigger=true when sessions >= threshold")
	}
}

func TestShouldTrigger_ThresholdExceeded(t *testing.T) {
	t.Parallel()

	config := settings.EvolveSettings{Enabled: true, SessionThreshold: 5}
	state := evolve.State{SessionsSinceLastRun: 7}

	if !evolve.ShouldTrigger(config, state) {
		t.Error("expected ShouldTrigger=true when sessions > threshold")
	}
}

func TestShouldTrigger_ThresholdNotMet(t *testing.T) {
	t.Parallel()

	config := settings.EvolveSettings{Enabled: true, SessionThreshold: 5}
	state := evolve.State{SessionsSinceLastRun: 3}

	if evolve.ShouldTrigger(config, state) {
		t.Error("expected ShouldTrigger=false when sessions < threshold")
	}
}

func TestShouldTrigger_Disabled(t *testing.T) {
	t.Parallel()

	config := settings.EvolveSettings{Enabled: false, SessionThreshold: 5}
	state := evolve.State{SessionsSinceLastRun: 10}

	if evolve.ShouldTrigger(config, state) {
		t.Error("expected ShouldTrigger=false when config is disabled")
	}
}

func TestIncrementSessionCount(t *testing.T) {
	t.Parallel()

	state := evolve.State{SessionsSinceLastRun: 3}
	evolve.IncrementSessionCount(&state)

	if state.SessionsSinceLastRun != 4 {
		t.Errorf("expected SessionsSinceLastRun=4, got %d", state.SessionsSinceLastRun)
	}
}

func TestRecordRun(t *testing.T) {
	t.Parallel()

	before := time.Now()
	state := evolve.State{
		SessionsSinceLastRun: 7,
		TotalRuns:            2,
		SuggestionsGenerated: 5,
	}
	evolve.RecordRun(&state, 3)
	after := time.Now()

	if state.SessionsSinceLastRun != 0 {
		t.Errorf("expected SessionsSinceLastRun=0, got %d", state.SessionsSinceLastRun)
	}
	if state.TotalRuns != 3 {
		t.Errorf("expected TotalRuns=3, got %d", state.TotalRuns)
	}
	if state.SuggestionsGenerated != 8 {
		t.Errorf("expected SuggestionsGenerated=8, got %d", state.SuggestionsGenerated)
	}
	if state.LastRunAt.Before(before) || state.LastRunAt.After(after) {
		t.Errorf("expected LastRunAt to be set to approximately now, got %v", state.LastRunAt)
	}
}
