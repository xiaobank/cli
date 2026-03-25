package evolve

import (
	"time"

	"github.com/entireio/cli/cmd/entire/cli/settings"
)

// ShouldTrigger returns true if the evolution loop should suggest running `entire improve`.
func ShouldTrigger(config settings.EvolveSettings, state State) bool {
	if !config.Enabled {
		return false
	}
	return state.SessionsSinceLastRun >= config.SessionThreshold
}

// IncrementSessionCount updates the state after a session ends.
func IncrementSessionCount(state *State) {
	state.SessionsSinceLastRun++
}

// RecordRun updates the state after an `entire improve` run completes.
func RecordRun(state *State, suggestionsGenerated int) {
	state.SessionsSinceLastRun = 0
	state.TotalRuns++
	state.SuggestionsGenerated += suggestionsGenerated
	state.LastRunAt = time.Now()
}
