package evolve_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/evolve"
	"github.com/entireio/cli/cmd/entire/cli/settings"
)

func TestCheckAndNotify_ThresholdMet(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	config := settings.EvolveSettings{Enabled: true, SessionThreshold: 5}
	state := evolve.State{SessionsSinceLastRun: 4} // will become 5 after increment

	evolve.CheckAndNotify(&buf, config, &state)

	if state.SessionsSinceLastRun != 5 {
		t.Errorf("expected SessionsSinceLastRun=5, got %d", state.SessionsSinceLastRun)
	}
	output := buf.String()
	if !strings.Contains(output, "entire improve") {
		t.Errorf("expected output to mention 'entire improve', got: %q", output)
	}
	if !strings.Contains(output, "5") {
		t.Errorf("expected output to mention session count 5, got: %q", output)
	}
}

func TestCheckAndNotify_BelowThreshold(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	config := settings.EvolveSettings{Enabled: true, SessionThreshold: 5}
	state := evolve.State{SessionsSinceLastRun: 2} // will become 3 after increment

	evolve.CheckAndNotify(&buf, config, &state)

	if state.SessionsSinceLastRun != 3 {
		t.Errorf("expected SessionsSinceLastRun=3, got %d", state.SessionsSinceLastRun)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output below threshold, got: %q", buf.String())
	}
}

func TestCheckAndNotify_Disabled(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	config := settings.EvolveSettings{Enabled: false, SessionThreshold: 5}
	state := evolve.State{SessionsSinceLastRun: 10}

	evolve.CheckAndNotify(&buf, config, &state)

	// Counter still increments even when disabled
	if state.SessionsSinceLastRun != 11 {
		t.Errorf("expected SessionsSinceLastRun=11, got %d", state.SessionsSinceLastRun)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output when disabled, got: %q", buf.String())
	}
}
