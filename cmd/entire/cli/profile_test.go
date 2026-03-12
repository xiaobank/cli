package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint/id"
	"github.com/entireio/cli/cmd/entire/cli/strategy"
	"github.com/entireio/cli/cmd/entire/cli/testutil"
	"github.com/go-git/go-git/v6"
)

func TestNewProfileCmd(t *testing.T) {
	t.Parallel()

	cmd := newProfileCmd()
	if cmd.Use != "profile" {
		t.Errorf("expected Use to be 'profile', got %q", cmd.Use)
	}

	if cmd.Flags().Lookup("checkpoint") == nil {
		t.Fatal("expected --checkpoint flag to exist")
	}
	if cmd.Flags().Lookup("session") == nil {
		t.Fatal("expected --session flag to exist")
	}
	if cmd.Flags().Lookup("commit") == nil {
		t.Fatal("expected --commit flag to exist")
	}
}

func TestRunProfile_OutputIncludesTurnDurations(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	testutil.WriteFile(t, tmpDir, "README.md", "init")
	testutil.GitAdd(t, tmpDir, "README.md")
	testutil.GitCommit(t, tmpDir, "initial commit")
	t.Chdir(tmpDir)

	repo, err := git.PlainOpen(tmpDir)
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	cpID := id.MustCheckpointID("abc123def456")
	transcriptData := []byte(strings.Join([]string{
		`{"type":"user","timestamp":"2026-03-12T10:00:00Z","message":{"content":"First prompt"}}`,
		`{"type":"assistant","timestamp":"2026-03-12T10:00:05Z","message":{"content":[{"type":"text","text":"Thinking"}]}}`,
		`{"type":"assistant","timestamp":"2026-03-12T10:00:09Z","message":{"content":[{"type":"text","text":"Done"}]}}`,
		`{"type":"user","timestamp":"2026-03-12T10:00:20Z","message":{"content":"Second prompt"}}`,
		`{"type":"assistant","timestamp":"2026-03-12T10:00:33Z","message":{"content":[{"type":"text","text":"Done2"}]}}`,
		`{"type":"user","timestamp":"2026-03-12T10:00:40Z","message":{"content":"Third prompt"}}`,
		"",
	}, "\n"))

	store := checkpoint.NewGitStore(repo)
	err = store.WriteCommitted(context.Background(), checkpoint.WriteCommittedOptions{
		CheckpointID:              cpID,
		SessionID:                 "session-1",
		Strategy:                  "manual-commit",
		Transcript:                transcriptData,
		FilesTouched:              []string{"README.md"},
		CheckpointsCount:          1,
		AuthorName:                "Test User",
		AuthorEmail:               "test@example.com",
		Agent:                     agent.AgentTypeClaudeCode,
		CheckpointTranscriptStart: 0,
	})
	if err != nil {
		t.Fatalf("WriteCommitted() error = %v", err)
	}

	var out bytes.Buffer
	err = runProfile(context.Background(), &out, "", "", "abc123")
	if err != nil {
		t.Fatalf("runProfile() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Checkpoint: abc123def456") {
		t.Errorf("expected checkpoint header, got:\n%s", output)
	}
	if !strings.Contains(output, "9s  First prompt") {
		t.Errorf("expected first turn duration, got:\n%s", output)
	}
	if !strings.Contains(output, "13s  Second prompt") {
		t.Errorf("expected second turn duration, got:\n%s", output)
	}
	if strings.Contains(output, "Third prompt") {
		t.Errorf("did not expect incomplete final turn in output, got:\n%s", output)
	}
}

func TestRunProfile_AmbiguousPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	testutil.WriteFile(t, tmpDir, "README.md", "init")
	testutil.GitAdd(t, tmpDir, "README.md")
	testutil.GitCommit(t, tmpDir, "initial commit")
	t.Chdir(tmpDir)

	repo, err := git.PlainOpen(tmpDir)
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}
	store := checkpoint.NewGitStore(repo)

	for _, cpID := range []id.CheckpointID{id.MustCheckpointID("abc123def456"), id.MustCheckpointID("abc999def456")} {
		err = store.WriteCommitted(context.Background(), checkpoint.WriteCommittedOptions{
			CheckpointID:              cpID,
			SessionID:                 "session-1",
			Strategy:                  "manual-commit",
			Transcript:                []byte("{\"type\":\"user\",\"timestamp\":\"2026-03-12T10:00:00Z\",\"message\":{\"content\":\"Prompt\"}}\n"),
			FilesTouched:              []string{"README.md"},
			CheckpointsCount:          1,
			AuthorName:                "Test User",
			AuthorEmail:               "test@example.com",
			Agent:                     agent.AgentTypeClaudeCode,
			CheckpointTranscriptStart: 0,
		})
		if err != nil {
			t.Fatalf("WriteCommitted(%s) error = %v", cpID, err)
		}
	}

	var out bytes.Buffer
	err = runProfile(context.Background(), &out, "", "", "abc")
	if err == nil {
		t.Fatal("expected ambiguous prefix error")
	}
	if !strings.Contains(err.Error(), "ambiguous checkpoint prefix") {
		t.Fatalf("expected ambiguous prefix error, got: %v", err)
	}
}

func TestRunProfile_RejectsMissingSelector(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := runProfile(context.Background(), &out, "", "", "")
	if err == nil {
		t.Fatal("expected error when no selector is provided")
	}
	if !strings.Contains(err.Error(), "must specify one of --session, --commit, --checkpoint") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunProfile_RejectsMultipleSelectors(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := runProfile(context.Background(), &out, "session-1", "", "abc123")
	if err == nil {
		t.Fatal("expected error when multiple selectors are provided")
	}
	if !strings.Contains(err.Error(), "cannot specify multiple") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProfileCmd_RejectsPositionalArgs(t *testing.T) {
	t.Parallel()

	cmd := newProfileCmd()
	cmd.SetArgs([]string{"abc123"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for positional args")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectLatestSessionPoint_PrefixMatchIncludesTemporary(t *testing.T) {
	t.Parallel()

	now := time.Now()
	points := []strategy.RewindPoint{
		{
			ID:           "aaaaaaaa",
			SessionID:    "ses_11111111",
			Date:         now.Add(-2 * time.Minute),
			CheckpointID: id.MustCheckpointID("abc123def456"),
			IsLogsOnly:   true,
		},
		{
			ID:         "bbbbbbbb",
			SessionID:  "ses_22222222",
			Date:       now.Add(-1 * time.Minute),
			IsLogsOnly: false,
		},
		{
			ID:           "cccccccc",
			SessionID:    "ses_11111111-live",
			Date:         now,
			CheckpointID: "",
			IsLogsOnly:   false,
		},
	}

	selected, found := selectLatestSessionPoint(points, "ses_11111111")
	if !found {
		t.Fatal("expected to find matching session point")
	}
	if selected.ID != "cccccccc" {
		t.Fatalf("expected newest matching point ID cccccccc, got %s", selected.ID)
	}
}

func TestProfileTurns_UnknownAgentFallsBackToOpenCodeJSON(t *testing.T) {
	t.Parallel()

	content := `{
		"messages": [
			{
				"info": {"role": "user", "time": {"created": 1773272328863}},
				"parts": [{"type": "text", "text": "First prompt"}]
			},
			{
				"info": {"role": "assistant", "time": {"created": 1773272328875, "completed": 1773272338775}},
				"parts": [{"type": "text", "text": "Done"}]
			},
			{
				"info": {"role": "user", "time": {"created": 1773272340000}},
				"parts": [{"type": "text", "text": "Second prompt"}]
			}
		]
	}`

	turns, err := profileTurns([]byte(content), agent.AgentTypeUnknown)
	if err != nil {
		t.Fatalf("profileTurns() error = %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 completed turn, got %d", len(turns))
	}
	if turns[0].prompt != "First prompt" {
		t.Fatalf("expected prompt 'First prompt', got %q", turns[0].prompt)
	}
	if turns[0].duration <= 0 {
		t.Fatalf("expected positive duration, got %s", turns[0].duration)
	}
}

func TestUnixTimestampAuto_Milliseconds(t *testing.T) {
	t.Parallel()

	ts := int64(1773272328863)
	got := unixTimestampAuto(ts)
	if got.IsZero() {
		t.Fatal("expected non-zero time for millisecond timestamp")
	}
	if got.UnixMilli() != ts {
		t.Fatalf("expected UnixMilli %d, got %d", ts, got.UnixMilli())
	}

	seconds := int64(1773272328)
	gotSeconds := unixTimestampAuto(seconds)
	if gotSeconds.Unix() != seconds {
		t.Fatalf("expected Unix %d, got %d", seconds, gotSeconds.Unix())
	}

	if !unixTimestampAuto(0).IsZero() {
		t.Fatal("expected zero timestamp for input 0")
	}

}
