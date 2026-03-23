//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/paths"
)

// TestFilter_CleanRepoPathsInTranscript verifies that absolute repo root paths
// in transcripts are normalized to __ent__/repo on the metadata branch after
// a checkpoint is condensed.
func TestFilter_CleanRepoPathsInTranscript(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	session := env.NewSession()

	// Simulate a prompt that mentions the repo root path
	if err := env.SimulateUserPromptSubmitWithPrompt(session.ID,
		"Edit the file at "+env.RepoDir+"/hello.go"); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	// Build a transcript that contains the repo root path
	env.WriteFile("hello.go", "package main\nfunc main() {}")
	session.TranscriptBuilder.AddUserMessage("Edit " + env.RepoDir + "/hello.go")
	session.TranscriptBuilder.AddAssistantMessage("I'll edit " + env.RepoDir + "/hello.go for you.")
	toolID := session.TranscriptBuilder.AddToolUse("mcp__acp__Write", env.RepoDir+"/hello.go", "package main\nfunc main() {}")
	session.TranscriptBuilder.AddToolResult(toolID)
	session.TranscriptBuilder.AddAssistantMessage("Done!")
	if err := session.TranscriptBuilder.WriteToFile(session.TranscriptPath); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Commit with shadow hooks to trigger condensation
	env.GitCommitWithShadowHooks("Add hello.go", "hello.go")

	// Read transcript from the metadata branch
	checkpointID := env.GetLatestCheckpointID()
	transcriptPath := checkpointID[:2] + "/" + checkpointID[2:] + "/0/" + paths.TranscriptFileName
	transcriptContent, found := env.ReadFileFromBranch(paths.MetadataBranchName, transcriptPath)
	if !found {
		t.Fatalf("transcript not found on %s at %s", paths.MetadataBranchName, transcriptPath)
	}

	// Verify repo root is replaced with __ent__/repo
	if strings.Contains(transcriptContent, env.RepoDir) {
		t.Errorf("transcript on metadata branch should not contain repo root %q, but does:\n%s",
			env.RepoDir, transcriptContent[:min(500, len(transcriptContent))])
	}
	if !strings.Contains(transcriptContent, "__ent__/repo") {
		t.Errorf("transcript on metadata branch should contain __ent__/repo placeholder, but doesn't:\n%s",
			transcriptContent[:min(500, len(transcriptContent))])
	}
}

// TestFilter_CleanPromptsOnMetadataBranch verifies that prompts stored on the
// metadata branch have absolute paths normalized.
func TestFilter_CleanPromptsOnMetadataBranch(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	session := env.NewSession()

	env.WriteFile("app.go", "package main\n// fixed")

	// Submit a prompt containing the repo root path
	prompt := "Please fix the bug in " + env.RepoDir + "/app.go"
	if err := env.SimulateUserPromptSubmitWithPrompt(session.ID, prompt); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	session.CreateTranscript(prompt, []FileChange{{Path: "app.go", Content: "package main\n// fixed"}})
	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Commit with shadow hooks to trigger condensation
	env.GitCommitWithShadowHooks("Fix app.go", "app.go")

	// Read prompt from the metadata branch
	checkpointID := env.GetLatestCheckpointID()
	promptPath := checkpointID[:2] + "/" + checkpointID[2:] + "/0/" + paths.PromptFileName
	promptContent, found := env.ReadFileFromBranch(paths.MetadataBranchName, promptPath)
	if !found {
		t.Fatalf("prompt not found on %s at %s", paths.MetadataBranchName, promptPath)
	}

	if strings.Contains(promptContent, env.RepoDir) {
		t.Errorf("prompt should not contain repo root %q, got: %s", env.RepoDir, promptContent)
	}
	if !strings.Contains(promptContent, "__ent__/repo") {
		t.Errorf("prompt should contain __ent__/repo, got: %s", promptContent)
	}
}

// TestFilter_SmudgeRestoresPathsOnLogsOnlyRewind verifies that when doing a
// logs-only rewind (from metadata branch), the transcript written to the agent's
// session file has __ent__/repo replaced back with the actual repo root path.
func TestFilter_SmudgeRestoresPathsOnLogsOnlyRewind(t *testing.T) {
	t.Parallel()
	env := NewFeatureBranchEnv(t)
	session := env.NewSession()

	env.WriteFile("main.go", "v1")

	if err := env.SimulateUserPromptSubmit(session.ID); err != nil {
		t.Fatalf("SimulateUserPromptSubmit failed: %v", err)
	}

	// Build transcript with repo root paths
	session.TranscriptBuilder.AddUserMessage("Edit " + env.RepoDir + "/main.go")
	session.TranscriptBuilder.AddAssistantMessage("Editing " + env.RepoDir + "/main.go")
	toolID := session.TranscriptBuilder.AddToolUse("mcp__acp__Write", env.RepoDir+"/main.go", "v1")
	session.TranscriptBuilder.AddToolResult(toolID)
	if err := session.TranscriptBuilder.WriteToFile(session.TranscriptPath); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	if err := env.SimulateStop(session.ID, session.TranscriptPath); err != nil {
		t.Fatalf("SimulateStop failed: %v", err)
	}

	// Commit to condensate checkpoint to metadata branch
	env.GitCommitWithShadowHooks("Add main.go", "main.go")

	// Delete the local transcript so we can verify it's restored from the metadata branch
	localTranscript := filepath.Join(env.ClaudeProjectDir, session.ID+".jsonl")
	os.Remove(localTranscript)

	// Make another commit so the checkpoint becomes a logs-only rewind point
	env.WriteFile("extra.go", "package main")
	env.GitCommitWithShadowHooks("Add extra.go", "extra.go")

	// Get rewind points and find the logs-only one
	points := env.GetRewindPoints()
	var logsOnlyPoint *RewindPoint
	for i := range points {
		if points[i].IsLogsOnly {
			logsOnlyPoint = &points[i]
			break
		}
	}
	if logsOnlyPoint == nil {
		t.Fatal("expected a logs-only rewind point after additional commit")
	}

	// Rewind logs-only to restore transcript from metadata branch
	if err := env.RewindLogsOnly(logsOnlyPoint.ID); err != nil {
		t.Fatalf("RewindLogsOnly failed: %v", err)
	}

	// Read the restored transcript
	restoredData, err := os.ReadFile(localTranscript)
	if err != nil {
		t.Fatalf("failed to read restored transcript at %s: %v", localTranscript, err)
	}
	restoredContent := string(restoredData)

	// Verify real paths are restored (smudge applied)
	if strings.Contains(restoredContent, "__ent__/repo") {
		t.Errorf("restored transcript should not contain __ent__/repo placeholder, but does:\n%s",
			restoredContent[:min(500, len(restoredContent))])
	}
	if !strings.Contains(restoredContent, env.RepoDir) {
		t.Errorf("restored transcript should contain actual repo root %q, but doesn't:\n%s",
			env.RepoDir, restoredContent[:min(500, len(restoredContent))])
	}
}
