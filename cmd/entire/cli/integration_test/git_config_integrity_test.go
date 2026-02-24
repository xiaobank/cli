//go:build integration

package integration

import (
	"os/exec"
	"strings"
	"testing"
)

// TestGitConfigNotCorruptedDuringSession verifies that a full session lifecycle
// (user-prompt-submit → file changes → stop) does not alter local .git/config.
func TestGitConfigNotCorruptedDuringSession(t *testing.T) {
	t.Parallel()
	RunForAllStrategies(t, func(t *testing.T, env *TestEnv, strategyName string) {
		// Snapshot local config before session
		nameBefore := getLocalGitConfig(t, env.RepoDir, "user.name")
		emailBefore := getLocalGitConfig(t, env.RepoDir, "user.email")

		// Verify precondition: local config must be set (InitRepo uses go-git SetConfig)
		// so the comparison is meaningful, not just empty == empty.
		if nameBefore == "" {
			t.Fatal("precondition failed: expected local user.name to be set by InitRepo")
		}
		if emailBefore == "" {
			t.Fatal("precondition failed: expected local user.email to be set by InitRepo")
		}

		// Run a full session: start → write file → stop
		session := env.NewSession()
		if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
			t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
		}

		env.WriteFile("session-file.txt", "created during session")

		session.CreateTranscript("Create a file", []FileChange{
			{Path: "session-file.txt", Content: "created during session"},
		})

		if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
			t.Fatalf("SimulateStop failed: %v", err)
		}

		// Snapshot local config after session
		nameAfter := getLocalGitConfig(t, env.RepoDir, "user.name")
		emailAfter := getLocalGitConfig(t, env.RepoDir, "user.email")

		// Assert config unchanged
		if nameBefore != nameAfter {
			t.Errorf("local git config user.name changed during session: %q → %q", nameBefore, nameAfter)
		}
		if emailBefore != emailAfter {
			t.Errorf("local git config user.email changed during session: %q → %q", emailBefore, emailAfter)
		}
	})
}

// TestGitConfigUserNameNotLiteralUserEmail is a specific regression test for #456.
// It verifies that user.name is never set to the literal string "user.email".
func TestGitConfigUserNameNotLiteralUserEmail(t *testing.T) {
	t.Parallel()
	RunForAllStrategies(t, func(t *testing.T, env *TestEnv, strategyName string) {
		// Run a full session
		session := env.NewSession()
		if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
			t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
		}

		env.WriteFile("test-file.txt", "test content")
		session.CreateTranscript("Create test file", []FileChange{
			{Path: "test-file.txt", Content: "test content"},
		})

		if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
			t.Fatalf("SimulateStop failed: %v", err)
		}

		// The specific #456 corruption: user.name set to literal "user.email"
		userName := getLocalGitConfig(t, env.RepoDir, "user.name")
		if userName == "user.email" {
			t.Errorf("REGRESSION #456: local git config user.name is the literal string \"user.email\"")
		}
	})
}

// getLocalGitConfig reads a value from local .git/config only.
// Returns empty string if not set.
func getLocalGitConfig(t *testing.T, repoDir, key string) string {
	t.Helper()
	cmd := exec.Command("git", "config", "--local", "--get", key)
	cmd.Dir = repoDir
	output, err := cmd.Output()
	if err != nil {
		// exit code 1 means key not found — that's fine
		return ""
	}
	return strings.TrimSpace(string(output))
}
