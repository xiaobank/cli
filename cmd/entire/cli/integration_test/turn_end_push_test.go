//go:build integration

package integration

import (
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/session"
)

const testRemoteName = "origin"

// TestShadow_TurnEndPushFlagLifecycle tests the full lifecycle of the
// PushedDuringTurnRemote flag through a complete agent turn:
//
// Flow:
// 1. Agent starts turn (ACTIVE)
// 2. Agent creates files, user commits -> PostCommit condenses checkpoint
// 3. Verify TurnCheckpointIDs is populated
// 4. Manually set PushedDuringTurnRemote (simulating PrePush)
// 5. Agent continues work, updates transcript
// 6. Agent stops -> HandleTurnEnd finalizes and clears flags
// 7. Verify both PushedDuringTurnRemote and TurnCheckpointIDs are cleared
func TestShadow_TurnEndPushFlagLifecycle(t *testing.T) {
	t.Parallel()

	env := NewFeatureBranchEnv(t)
	sess := env.NewSession()

	// Step 1: Start session (ACTIVE)
	if err := env.SimulateUserPromptSubmitWithPromptAndTranscriptPath(
		sess.ID, "Create feature function", sess.TranscriptPath,
	); err != nil {
		t.Fatalf("user-prompt-submit failed: %v", err)
	}

	state, err := env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState failed: %v", err)
	}
	if state.Phase != session.PhaseActive {
		t.Fatalf("Expected ACTIVE phase, got %s", state.Phase)
	}

	// Step 2: Create file, transcript, and commit
	env.WriteFile("feature.go", "package main\n\nfunc Feature() {}\n")
	sess.CreateTranscript("Create feature function", []FileChange{
		{Path: "feature.go", Content: "package main\n\nfunc Feature() {}\n"},
	})

	env.GitCommitWithShadowHooks("Add feature", "feature.go")

	// Step 3: Verify TurnCheckpointIDs is populated after PostCommit
	state, err = env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState after commit failed: %v", err)
	}
	if len(state.TurnCheckpointIDs) == 0 {
		t.Fatal("TurnCheckpointIDs should be populated after PostCommit condensation")
	}
	t.Logf("TurnCheckpointIDs after commit: %v", state.TurnCheckpointIDs)

	// Verify PushedDuringTurnRemote is initially empty (no push has happened)
	if state.PushedDuringTurnRemote != "" {
		t.Errorf("PushedDuringTurnRemote should be empty before any push, got %q",
			state.PushedDuringTurnRemote)
	}

	// Step 4: Simulate PrePush setting the flag
	state.PushedDuringTurnRemote = testRemoteName
	if err := env.WriteSessionState(sess.ID, state); err != nil {
		t.Fatalf("WriteSessionState failed: %v", err)
	}

	// Verify the flag was persisted
	state, err = env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState after flag set failed: %v", err)
	}
	if state.PushedDuringTurnRemote != testRemoteName {
		t.Fatalf("PushedDuringTurnRemote should be 'origin' after write, got %q",
			state.PushedDuringTurnRemote)
	}

	// Step 5: Agent continues work - update transcript with more content
	sess.TranscriptBuilder.AddUserMessage("Also add a helper function")
	sess.TranscriptBuilder.AddAssistantMessage("Adding helper function now")
	toolID := sess.TranscriptBuilder.AddToolUse("mcp__acp__Write", "helper.go", "package main\n\nfunc Helper() {}\n")
	sess.TranscriptBuilder.AddToolResult(toolID)
	sess.TranscriptBuilder.AddAssistantMessage("Done with both changes!")
	if err := sess.TranscriptBuilder.WriteToFile(sess.TranscriptPath); err != nil {
		t.Fatalf("Failed to write updated transcript: %v", err)
	}

	// Step 6: Agent stops - HandleTurnEnd should finalize and clear flags
	if err := env.SimulateStop(sess.ID, sess.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Step 7: Verify both flags are cleared
	state, err = env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState after stop failed: %v", err)
	}

	if state.PushedDuringTurnRemote != "" {
		t.Errorf("PushedDuringTurnRemote should be cleared after HandleTurnEnd, got %q",
			state.PushedDuringTurnRemote)
	}
	if len(state.TurnCheckpointIDs) != 0 {
		t.Errorf("TurnCheckpointIDs should be cleared after HandleTurnEnd, got %v",
			state.TurnCheckpointIDs)
	}
	if state.Phase != session.PhaseIdle {
		t.Errorf("Expected IDLE phase after stop, got %s", state.Phase)
	}
}

// TestShadow_TurnEndNoPushWhenNotFlagged tests that HandleTurnEnd does not
// attempt any push-related behavior when PushedDuringTurnRemote is not set.
//
// Flow:
// 1. Agent starts turn, creates file, commits
// 2. PushedDuringTurnRemote is NOT set (no push happened)
// 3. Agent stops
// 4. Verify PushedDuringTurnRemote remains empty
// 5. Verify TurnCheckpointIDs is still cleared (normal finalization)
func TestShadow_TurnEndNoPushWhenNotFlagged(t *testing.T) {
	t.Parallel()

	env := NewFeatureBranchEnv(t)
	sess := env.NewSession()

	// Start session (ACTIVE)
	if err := env.SimulateUserPromptSubmitWithPromptAndTranscriptPath(
		sess.ID, "Create utility", sess.TranscriptPath,
	); err != nil {
		t.Fatalf("user-prompt-submit failed: %v", err)
	}

	// Create file, transcript, and commit
	env.WriteFile("util.go", "package main\n\nfunc Util() {}\n")
	sess.CreateTranscript("Create utility", []FileChange{
		{Path: "util.go", Content: "package main\n\nfunc Util() {}\n"},
	})

	env.GitCommitWithShadowHooks("Add utility", "util.go")

	// Verify TurnCheckpointIDs is populated (condensation happened)
	state, err := env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState after commit failed: %v", err)
	}
	if len(state.TurnCheckpointIDs) == 0 {
		t.Fatal("TurnCheckpointIDs should be populated after commit")
	}

	// Do NOT set PushedDuringTurnRemote - this is the key difference from lifecycle test
	if state.PushedDuringTurnRemote != "" {
		t.Fatalf("PushedDuringTurnRemote should be empty (no push), got %q",
			state.PushedDuringTurnRemote)
	}

	// Stop the session
	if err := env.SimulateStop(sess.ID, sess.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Verify state after stop
	state, err = env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState after stop failed: %v", err)
	}

	if state.PushedDuringTurnRemote != "" {
		t.Errorf("PushedDuringTurnRemote should remain empty when not flagged, got %q",
			state.PushedDuringTurnRemote)
	}
	if len(state.TurnCheckpointIDs) != 0 {
		t.Errorf("TurnCheckpointIDs should be cleared after stop, got %v",
			state.TurnCheckpointIDs)
	}
	if state.Phase != session.PhaseIdle {
		t.Errorf("Expected IDLE phase after stop, got %s", state.Phase)
	}
}

// TestShadow_NewPromptClearsPushedDuringTurnRemote tests that InitializeSession
// clears PushedDuringTurnRemote when a new prompt starts. This ensures stale
// push flags from a previous turn do not carry over.
//
// Flow:
// 1. Agent starts turn, creates file, commits
// 2. Set PushedDuringTurnRemote = "origin" (simulating PrePush)
// 3. Agent stops
// 4. Start a new prompt (same session ID)
// 5. Verify PushedDuringTurnRemote is cleared by InitializeSession
func TestShadow_NewPromptClearsPushedDuringTurnRemote(t *testing.T) {
	t.Parallel()

	env := NewFeatureBranchEnv(t)
	sess := env.NewSession()

	// Start session (ACTIVE)
	if err := env.SimulateUserPromptSubmitWithPromptAndTranscriptPath(
		sess.ID, "Create service", sess.TranscriptPath,
	); err != nil {
		t.Fatalf("user-prompt-submit failed: %v", err)
	}

	// Create file, transcript, and commit
	env.WriteFile("service.go", "package main\n\nfunc Service() {}\n")
	sess.CreateTranscript("Create service", []FileChange{
		{Path: "service.go", Content: "package main\n\nfunc Service() {}\n"},
	})

	env.GitCommitWithShadowHooks("Add service", "service.go")

	// Set PushedDuringTurnRemote (simulating PrePush)
	state, err := env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState after commit failed: %v", err)
	}
	state.PushedDuringTurnRemote = testRemoteName
	if err := env.WriteSessionState(sess.ID, state); err != nil {
		t.Fatalf("WriteSessionState failed: %v", err)
	}

	// Stop the session (transitions to IDLE, clears PushedDuringTurnRemote)
	if err := env.SimulateStop(sess.ID, sess.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Verify stop cleared the flag
	state, err = env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState after stop failed: %v", err)
	}
	if state.PushedDuringTurnRemote != "" {
		t.Errorf("PushedDuringTurnRemote should be cleared after stop, got %q",
			state.PushedDuringTurnRemote)
	}

	// Now set PushedDuringTurnRemote again to simulate a leftover from an
	// incomplete turn (e.g., crash after push but before stop cleared it)
	state.PushedDuringTurnRemote = testRemoteName
	if err := env.WriteSessionState(sess.ID, state); err != nil {
		t.Fatalf("WriteSessionState for leftover flag failed: %v", err)
	}

	// Start a new prompt (InitializeSession should clear the flag)
	if err := env.SimulateUserPromptSubmitWithPromptAndTranscriptPath(
		sess.ID, "Second prompt", sess.TranscriptPath,
	); err != nil {
		t.Fatalf("second user-prompt-submit failed: %v", err)
	}

	// Verify InitializeSession cleared PushedDuringTurnRemote
	state, err = env.GetSessionState(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionState after second prompt failed: %v", err)
	}
	if state.PushedDuringTurnRemote != "" {
		t.Errorf("PushedDuringTurnRemote should be cleared by InitializeSession on new prompt, got %q",
			state.PushedDuringTurnRemote)
	}
	if state.Phase != session.PhaseActive {
		t.Errorf("Expected ACTIVE phase after new prompt, got %s", state.Phase)
	}

	// TurnCheckpointIDs should also be cleared for the new turn
	if len(state.TurnCheckpointIDs) != 0 {
		t.Errorf("TurnCheckpointIDs should be cleared on new prompt, got %v",
			state.TurnCheckpointIDs)
	}
}
