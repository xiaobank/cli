package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/strategy"
	"github.com/entireio/cli/cmd/entire/cli/testutil"
	"github.com/entireio/cli/cmd/entire/cli/trailers"

	"github.com/go-git/go-git/v6"
	"github.com/stretchr/testify/require"
)

func TestCommit_WritesCheckpointTrailerAndExtraHeader(t *testing.T) {
	repoDir := t.TempDir()
	testutil.InitRepo(t, repoDir)
	testutil.WriteFile(t, repoDir, "hello.txt", "initial\n")
	testutil.GitAdd(t, repoDir, "hello.txt")
	testutil.GitCommit(t, repoDir, "initial")

	t.Chdir(repoDir)
	testutil.WriteFile(t, repoDir, "hello.txt", "agent change\n")
	testutil.GitAdd(t, repoDir, "hello.txt")

	sessionID := "test-commit-session"
	require.NoError(t, setupCommitSession(context.Background(), repoDir, sessionID, "hello.txt", "agent change\n"))

	cmd := newCommitCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"-m", "feat: add header"})

	require.NoError(t, cmd.Execute())

	repo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)

	head, err := repo.Head()
	require.NoError(t, err)

	commit, err := repo.CommitObject(head.Hash())
	require.NoError(t, err)

	cpID, found := trailers.ParseCheckpoint(commit.Message)
	require.True(t, found, "expected checkpoint trailer in commit message")

	require.Len(t, commit.ExtraHeaders, 1)
	require.Equal(t, trailers.CheckpointTrailerKey, commit.ExtraHeaders[0].Key)
	require.Equal(t, cpID.String(), commit.ExtraHeaders[0].Value)

	require.Contains(t, out.String(), "[")
	require.Contains(t, out.String(), "feat: add header")
}

func TestCommit_NoStagedChangesReturnsError(t *testing.T) {
	repoDir := t.TempDir()
	testutil.InitRepo(t, repoDir)
	testutil.WriteFile(t, repoDir, "hello.txt", "initial\n")
	testutil.GitAdd(t, repoDir, "hello.txt")
	testutil.GitCommit(t, repoDir, "initial")

	t.Chdir(repoDir)

	cmd := newCommitCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"-m", "feat: empty"})

	err := cmd.Execute()
	require.Error(t, err)
	require.True(t, errors.Is(err, git.ErrEmptyCommit))
}

func setupCommitSession(ctx context.Context, repoDir, sessionID, filePath, content string) error {
	strat := strategy.NewManualCommitStrategy()
	if err := strat.InitializeSession(ctx, sessionID, agent.AgentTypeClaudeCode, "", "update file", ""); err != nil {
		return err
	}

	metadataDir := filepath.Join(".entire", "metadata", sessionID)
	metadataDirAbs := filepath.Join(repoDir, metadataDir)
	if err := os.MkdirAll(metadataDirAbs, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(metadataDirAbs, "prompt.txt"), []byte("update file\n"), 0o644); err != nil {
		return err
	}

	return strat.SaveStep(ctx, strategy.StepContext{
		SessionID:      sessionID,
		ModifiedFiles:  []string{filePath},
		MetadataDir:    metadataDir,
		MetadataDirAbs: metadataDirAbs,
		CommitMessage:  "checkpoint",
		AuthorName:     "Test User",
		AuthorEmail:    "test@example.com",
		AgentType:      agent.AgentTypeClaudeCode,
	})
}

func TestCommit_RegisteredOnRoot(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	cmd, _, err := root.Find([]string{"commit"})
	require.NoError(t, err)
	require.NotNil(t, cmd)
	require.Equal(t, "commit", cmd.Name())
}
