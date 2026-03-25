package evolve

import (
	"fmt"
	"io"

	"github.com/entireio/cli/cmd/entire/cli/settings"
)

// CheckAndNotify increments the session counter and notifies the user
// when the evolution threshold is reached. Called after session condensation.
func CheckAndNotify(w io.Writer, config settings.EvolveSettings, state *State) {
	IncrementSessionCount(state)
	if !ShouldTrigger(config, *state) {
		return
	}
	fmt.Fprintf(w, "\n  Tip: %d sessions since last improvement analysis.\n", state.SessionsSinceLastRun)
	fmt.Fprintln(w, "  Run `entire improve` to get context file suggestions.")
}
