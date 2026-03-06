//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/session"
)

// TestPostFileEdit_TracksEditedFiles tests the end-to-end flow of file tracking
// via the post-file-edit hook. When the agent edits files (Write/Edit tools),
// the hook handler normalizes absolute paths to repo-relative and appends them
// to .git/entire-sessions/<session-id>.files.
func TestPostFileEdit_TracksEditedFiles(t *testing.T) {
	t.Parallel()

	env := NewFeatureBranchEnv(t)
	sess := env.NewSession()

	// Start session (user-prompt-submit creates session state and starts a turn)
	if err := env.SimulateUserPromptSubmit(sess.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	// Simulate two file edits with absolute paths (the handler normalizes them)
	if err := env.SimulatePostFileEdit(PostFileEditInput{
		SessionID:      sess.ID,
		TranscriptPath: sess.TranscriptPath,
		ToolUseID:      "tu_write_1",
		FilePath:       filepath.Join(env.RepoDir, "src", "main.go"),
	}); err != nil {
		t.Fatalf("SimulatePostFileEdit (src/main.go) failed: %v", err)
	}

	if err := env.SimulatePostFileEdit(PostFileEditInput{
		SessionID:      sess.ID,
		TranscriptPath: sess.TranscriptPath,
		ToolUseID:      "tu_edit_1",
		FilePath:       filepath.Join(env.RepoDir, "README.md"),
	}); err != nil {
		t.Fatalf("SimulatePostFileEdit (README.md) failed: %v", err)
	}

	// Verify via ReadFilesTouched (deduplicates and sorts)
	stateDir := filepath.Join(env.RepoDir, ".git", "entire-sessions")
	store := session.NewStateStoreWithDir(stateDir)
	files, err := store.ReadFilesTouched(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("ReadFilesTouched failed: %v", err)
	}

	expected := []string{"README.md", "src/main.go"}
	if len(files) != len(expected) {
		t.Fatalf("Expected %d tracked files, got %d: %v", len(expected), len(files), files)
	}
	for i, want := range expected {
		if files[i] != want {
			t.Errorf("Tracked file [%d]: want %q, got %q", i, want, files[i])
		}
	}
}
