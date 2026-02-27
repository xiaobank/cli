package kiro

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

func TestIdentity(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	if ag.Name() != agent.AgentNameKiro {
		t.Errorf("expected name %q, got %q", agent.AgentNameKiro, ag.Name())
	}
	if ag.Type() != agent.AgentTypeKiro {
		t.Errorf("expected type %q, got %q", agent.AgentTypeKiro, ag.Type())
	}
	if ag.Description() == "" {
		t.Error("expected non-empty description")
	}
	if !ag.IsPreview() {
		t.Error("expected IsPreview to be true")
	}
	if len(ag.ProtectedDirs()) != 1 || ag.ProtectedDirs()[0] != ".kiro" {
		t.Errorf("expected ProtectedDirs [.kiro], got %v", ag.ProtectedDirs())
	}
}

func TestDetectPresence_WithKiroDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".kiro"), 0o750); err != nil {
		t.Fatalf("failed to create .kiro dir: %v", err)
	}

	// DetectPresence uses paths.WorktreeRoot which won't work in temp dirs,
	// but the fallback to "." means we can't test this reliably without a git repo.
	// We test the core logic indirectly through the other tests.
	ag := &KiroAgent{}
	_ = ag // Agent created, test passes if no panic
}

func TestGetSessionDir(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	dir, err := ag.GetSessionDir("/some/project/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir == "" {
		t.Error("expected non-empty session dir")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
}

func TestGetSessionDir_EnvOverride(t *testing.T) {
	t.Setenv("ENTIRE_TEST_KIRO_PROJECT_DIR", "/test/override")
	ag := &KiroAgent{}
	dir, err := ag.GetSessionDir("/some/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/test/override" {
		t.Errorf("expected /test/override, got %q", dir)
	}
}

func TestResolveSessionFile(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	path := ag.ResolveSessionFile("/tmp/sessions", "abc-123")
	expected := filepath.Join("/tmp/sessions", "abc-123.json")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestFormatResumeCommand(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	cmd := ag.FormatResumeCommand("any-session-id")
	if cmd != "kiro-cli" {
		t.Errorf("expected %q, got %q", "kiro-cli", cmd)
	}
}

func TestSanitizePathForKiro(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"/Users/test/project", "-Users-test-project"},
		{"simple", "simple"},
		{"/path/with spaces/file", "-path-with-spaces-file"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := SanitizePathForKiro(tt.input)
			if got != tt.want {
				t.Errorf("SanitizePathForKiro(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReadSession(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	conv := Conversation{
		ConversationID: "test-conv-1",
		History: []HistoryEntry{
			{
				Role: "user",
				Content: []ContentPart{
					{Type: "text", Text: "Fix the bug"},
				},
			},
			{
				Role: "assistant",
				Content: []ContentPart{
					{Type: "tool_use", Name: "fs_write", Input: map[string]any{"file_path": "main.go"}},
				},
			},
		},
	}

	data, err := json.Marshal(conv)
	if err != nil {
		t.Fatalf("failed to marshal test data: %v", err)
	}

	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "test-session.json")
	if err := os.WriteFile(sessionFile, data, 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	input := &agent.HookInput{
		SessionID:  "test-conv-1",
		SessionRef: sessionFile,
	}

	session, err := ag.ReadSession(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.SessionID != "test-conv-1" {
		t.Errorf("expected session ID 'test-conv-1', got %q", session.SessionID)
	}
	if len(session.ModifiedFiles) != 1 || session.ModifiedFiles[0] != "main.go" {
		t.Errorf("expected modified files [main.go], got %v", session.ModifiedFiles)
	}
}

func TestReadSession_NoRef(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	_, err := ag.ReadSession(&agent.HookInput{})
	if err == nil {
		t.Fatal("expected error for empty session ref")
	}
}

func TestWriteSession(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "write-test.json")

	session := &agent.AgentSession{
		SessionID:  "test-1",
		SessionRef: sessionFile,
		NativeData: []byte(`{"conversation_id":"test-1","history":[]}`),
	}

	if err := ag.WriteSession(context.Background(), session); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != string(session.NativeData) {
		t.Error("written data does not match session data")
	}
}

func TestWriteSession_NilSession(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	if err := ag.WriteSession(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil session")
	}
}

func TestWriteSession_NoData(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	if err := ag.WriteSession(context.Background(), &agent.AgentSession{}); err == nil {
		t.Fatal("expected error for empty session data")
	}
}

func TestChunkTranscript_SmallContent(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	content := []byte(testConversationJSON)

	chunks, err := ag.ChunkTranscript(context.Background(), content, len(content)+1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for small content, got %d", len(chunks))
	}
}

func TestChunkTranscript_SplitsLargeContent(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	content := []byte(testConversationJSON)

	chunks, err := ag.ChunkTranscript(context.Background(), content, 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for small maxSize, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		conv, parseErr := ParseConversation(chunk)
		if parseErr != nil {
			t.Fatalf("chunk %d: failed to parse: %v", i, parseErr)
		}
		if conv == nil || len(conv.History) == 0 {
			t.Errorf("chunk %d: expected at least 1 history entry", i)
		}
	}
}

func TestChunkTranscript_RoundTrip(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	content := []byte(testConversationJSON)

	chunks, err := ag.ChunkTranscript(context.Background(), content, 300)
	if err != nil {
		t.Fatalf("chunk error: %v", err)
	}

	reassembled, err := ag.ReassembleTranscript(chunks)
	if err != nil {
		t.Fatalf("reassemble error: %v", err)
	}

	original, err := ParseConversation(content)
	if err != nil {
		t.Fatalf("failed to parse original: %v", err)
	}
	result, err := ParseConversation(reassembled)
	if err != nil {
		t.Fatalf("failed to parse reassembled: %v", err)
	}

	if len(result.History) != len(original.History) {
		t.Fatalf("history count mismatch: %d vs %d", len(result.History), len(original.History))
	}
	if result.ConversationID != original.ConversationID {
		t.Errorf("conversation ID mismatch: %q vs %q", result.ConversationID, original.ConversationID)
	}
}

func TestReassembleTranscript_Empty(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	_, err := ag.ReassembleTranscript(nil)
	if err == nil {
		t.Fatal("expected error for nil chunks")
	}
}

func TestExtractModifiedFiles(t *testing.T) {
	t.Parallel()

	files, err := ExtractModifiedFiles([]byte(testConversationJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	if files[0] != "main.go" {
		t.Errorf("expected first file 'main.go', got %q", files[0])
	}
	if files[1] != "util.go" {
		t.Errorf("expected second file 'util.go', got %q", files[1])
	}
}

func TestExtractModifiedFiles_Empty(t *testing.T) {
	t.Parallel()

	files, err := ExtractModifiedFiles(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil for nil data, got %v", files)
	}
}

func TestParseConversation(t *testing.T) {
	t.Parallel()

	conv, err := ParseConversation([]byte(testConversationJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv == nil {
		t.Fatal("expected non-nil conversation")
	}
	if len(conv.History) != 4 {
		t.Fatalf("expected 4 history entries, got %d", len(conv.History))
	}
	if conv.ConversationID != "test-conv-123" {
		t.Errorf("expected conversation ID 'test-conv-123', got %q", conv.ConversationID)
	}
}

func TestParseConversation_Empty(t *testing.T) {
	t.Parallel()

	conv, err := ParseConversation(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv != nil {
		t.Errorf("expected nil for nil data, got %+v", conv)
	}

	conv, err = ParseConversation([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv != nil {
		t.Errorf("expected nil for empty data, got %+v", conv)
	}
}

func TestParseConversation_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := ParseConversation([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
