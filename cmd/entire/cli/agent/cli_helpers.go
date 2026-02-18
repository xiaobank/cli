package agent

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// CLIResponse represents the JSON response from agent CLIs that support --output-format json.
// Used by both wingman review and summarization.
type CLIResponse struct {
	Result string `json:"result"`
}

// StripGitEnv returns a copy of env with all GIT_* variables removed and
// agent-specific nesting-detection variables unset (e.g., CLAUDECODE).
// GIT_* removal prevents a subprocess from discovering or modifying the
// parent's git repo. CLAUDECODE removal prevents the Claude CLI from
// refusing to start due to nested-session detection.
func StripGitEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_") || strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// FormatExecError returns a human-readable error message for exec failures.
// Handles both "command not found" (exec.Error) and non-zero exit (exec.ExitError).
func FormatExecError(err error, cliName, stderr string) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return fmt.Errorf("%s CLI not found: %w", cliName, err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%s CLI failed (exit %d): %s", cliName, exitErr.ExitCode(), stderr)
	}
	return fmt.Errorf("failed to run %s CLI: %w", cliName, err)
}
