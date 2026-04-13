package strategy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/agent/types"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint/id"
	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/entireio/cli/cmd/entire/cli/session"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAgentWithTranscriptAnalyzer extends fakeExternalAgent with TranscriptAnalyzer
// support. The modifiedFiles field controls what ExtractModifiedFilesFromOffset returns,
// allowing tests to simulate "no new file modifications since last condensation."
type fakeAgentWithTranscriptAnalyzer struct {
	fakeExternalAgent

	transcript    []byte
	modifiedFiles []string // files returned by ExtractModifiedFilesFromOffset
}

func (f *fakeAgentWithTranscriptAnalyzer) ReadTranscript(_ string) ([]byte, error) {
	return f.transcript, nil
}

func (f *fakeAgentWithTranscriptAnalyzer) GetTranscriptPosition(_ string) (int, error) { //nolint:unparam // interface conformance
	return len(f.transcript), nil
}

func (f *fakeAgentWithTranscriptAnalyzer) ExtractModifiedFilesFromOffset(_ string, _ int) ([]string, int, error) { //nolint:unparam // interface conformance
	return f.modifiedFiles, 0, nil
}

func (f *fakeAgentWithTranscriptAnalyzer) ExtractPrompts(_ string, _ int) ([]string, error) {
	return nil, nil
}

func (f *fakeAgentWithTranscriptAnalyzer) ExtractSummary(_ string) (string, error) {
	return "", nil
}

// TestPostCommit_ReadOnlyHeuristic_BlocksActiveSession verifies that the
// read-only session heuristic in shouldCondenseWithOverlapCheck incorrectly
// skips condensation for an ACTIVE session when a concurrent session exists.
//
// Bug: When an ACTIVE session makes multiple commits during a single turn,
// the first commit condenses successfully and clears state.FilesTouched.
// On subsequent commits, filesTouchedBefore is resolved via transcript
// extraction from CheckpointTranscriptStart — but if the agent modified
// files before the first commit and only performed reads between commits,
// extraction returns empty. If a concurrent session has FilesTouched
// overlapping the committed files, sessionsWithCommittedFiles > 0 while
// filesTouchedBefore is empty, triggering the read-only heuristic which
// skips condensation. The prepare-commit-msg fast path already added the
// Entire-Checkpoint trailer unconditionally (ACTIVE + no TTY), so the
// commit has a dangling trailer pointing to a checkpoint that was never
// written to entire/checkpoints/v1, making it invisible to entire explain.
//
// Scenario:
//  1. Session A (any agent, IDLE) — has FilesTouched from a previous SaveStep
//  2. Session B (any agent, ACTIVE) — mid-turn, FilesTouched cleared by prior condensation
//  3. Session B commits files that overlap with session A's FilesTouched
//  4. PostCommit: sessionsWithCommittedFiles > 0 (from session A's state.FilesTouched)
//  5. PostCommit: session B's filesTouchedBefore is empty (no new Write/Edit in
//     transcript since the last condensation offset)
//  6. shouldCondenseWithOverlapCheck returns false → condensation skipped
func TestPostCommit_ReadOnlyHeuristic_BlocksActiveSession(t *testing.T) {
	dir := setupGitRepo(t)
	t.Chdir(dir)

	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)

	// Register a fake agent with transcript_analyzer support.
	const agentBName types.AgentName = "fake-agent-b"
	const agentBType types.AgentType = "Fake Agent B"
	transcriptContent := []byte(testTranscriptPromptResponse)
	fakeAgent := &fakeAgentWithTranscriptAnalyzer{
		fakeExternalAgent: fakeExternalAgent{name: agentBName, agentType: agentBType},
		transcript:        transcriptContent,
		modifiedFiles:     nil, // No new file modifications since last condensation offset
	}
	agent.Register(agentBName, func() agent.Agent { return fakeAgent })

	s := &ManualCommitStrategy{}

	worktreePath, err := paths.WorktreeRoot(context.Background())
	require.NoError(t, err)
	worktreeID, err := paths.GetWorktreeID(worktreePath)
	require.NoError(t, err)

	head, err := repo.Head()
	require.NoError(t, err)

	now := time.Now()
	baseCommit := head.Hash().String()

	// --- Session A: concurrent IDLE session with FilesTouched overlapping committed files ---
	// This simulates a previous agent session that tracked the same file.
	// LastCheckpointID prevents garbage collection by listAllSessionStates.
	sessionA := "session-a-concurrent"
	stateA := &SessionState{
		SessionID:           sessionA,
		BaseCommit:          baseCommit,
		WorktreePath:        worktreePath,
		WorktreeID:          worktreeID,
		StartedAt:           now.Add(-1 * time.Hour),
		Phase:               session.PhaseIdle,
		LastInteractionTime: &now,
		AgentType:           agent.AgentTypeClaudeCode,
		FilesTouched:        []string{"test.txt"},                // Claims the committed file
		LastCheckpointID:    id.MustCheckpointID("bbbbbbbbbbbb"), // Prevents GC by listAllSessionStates
	}
	require.NoError(t, s.saveSessionState(context.Background(), stateA))

	// --- Session B: ACTIVE session, mid-turn, FilesTouched cleared by prior condensation ---
	sessionB := "session-b-active"

	// Write transcript to disk
	transcriptPath := filepath.Join(dir, ".agent", "sessions", "transcript.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(transcriptPath), 0o755))
	require.NoError(t, os.WriteFile(transcriptPath, transcriptContent, 0o644))

	// Create prompt file so condensation has prompt data
	promptDir := filepath.Join(dir, ".entire", "metadata", sessionB)
	require.NoError(t, os.MkdirAll(promptDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(promptDir, "prompt.txt"), []byte("test prompt"), 0o644))

	stateB := &SessionState{
		SessionID:                 sessionB,
		BaseCommit:                baseCommit,
		WorktreePath:              worktreePath,
		WorktreeID:                worktreeID,
		StartedAt:                 now,
		Phase:                     session.PhaseActive,
		LastInteractionTime:       &now,
		AgentType:                 agentBType,
		TranscriptPath:            transcriptPath,
		CheckpointTranscriptStart: 2,   // Offset past current content (simulates prior condensation)
		FilesTouched:              nil, // Cleared by prior condensation
	}
	require.NoError(t, s.saveSessionState(context.Background(), stateB))

	// Commit with checkpoint trailer (simulates the ACTIVE agent committing mid-turn)
	checkpointIDStr := "a1b2c3d4e5f6"
	commitWithCheckpointTrailer(t, repo, dir, checkpointIDStr)

	// Run PostCommit
	err = s.PostCommit(context.Background())
	require.NoError(t, err)

	// Verify session B's checkpoint was condensed to entire/checkpoints/v1
	sessionsRef, err := repo.Reference(plumbing.NewBranchReferenceName(paths.MetadataBranchName), true)
	require.NoError(t, err, "entire/checkpoints/v1 branch should exist — "+
		"ACTIVE session should be condensed even when another session claims the committed files")
	assert.NotNil(t, sessionsRef)

	// Verify the checkpoint is findable (this is what 'entire explain' uses)
	store := checkpoint.NewGitStore(repo)
	committed, err := store.ListCommitted(context.Background())
	require.NoError(t, err)

	cpID := id.MustCheckpointID(checkpointIDStr)

	var found bool
	for _, c := range committed {
		if c.CheckpointID == cpID {
			found = true
			break
		}
	}
	assert.True(t, found, "checkpoint %s should be findable on entire/checkpoints/v1 — "+
		"this is what 'entire explain' uses to resolve Entire-Checkpoint trailers", checkpointIDStr)

	// Verify session B's state was updated (condensation succeeded)
	updatedB, err := s.loadSessionState(context.Background(), sessionB)
	require.NoError(t, err)
	require.NotNil(t, updatedB)
	assert.Equal(t, session.PhaseActive, updatedB.Phase)
	assert.Equal(t, checkpointIDStr, updatedB.LastCheckpointID.String(),
		"LastCheckpointID should be set after successful condensation")
}
