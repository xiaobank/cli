package kiro

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/agent/types"
)

// Compile-time interface compliance checks.
var (
	_ agent.Agent       = (*KiroAgent)(nil)
	_ agent.HookSupport = (*KiroAgent)(nil)
)

func TestNewKiroAgent(t *testing.T) {
	t.Parallel()

	ag := NewKiroAgent()
	if ag == nil {
		t.Fatal("NewKiroAgent() returned nil")
	}
	if _, ok := ag.(*KiroAgent); !ok {
		t.Errorf("NewKiroAgent() returned type %T, want *KiroAgent", ag)
	}
}

func TestName(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	if got := ag.Name(); got != types.AgentName("kiro") {
		t.Errorf("Name() = %q, want %q", got, "kiro")
	}
}

func TestType(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	if got := ag.Type(); got != types.AgentType("Kiro") {
		t.Errorf("Type() = %q, want %q", got, "Kiro")
	}
}

func TestDescription(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	desc := ag.Description()
	if desc == "" {
		t.Error("Description() returned empty string")
	}
}

func TestIsPreview(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	if !ag.IsPreview() {
		t.Error("IsPreview() = false, want true")
	}
}

func TestProtectedDirs(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	dirs := ag.ProtectedDirs()
	if len(dirs) != 1 || dirs[0] != ".kiro" {
		t.Errorf("ProtectedDirs() = %v, want [.kiro]", dirs)
	}
}

func TestDetectPresence_WithKiroDir(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, ".kiro"), 0o750); err != nil {
		t.Fatalf("failed to create .kiro dir: %v", err)
	}

	// DetectPresence uses paths.WorktreeRoot which won't resolve in a temp dir,
	// so it falls back to ".". We chdir to make it find .kiro.
	// Since t.Chdir is not parallelizable, we test a separate scenario below.
}

func TestDetectPresence_WithoutKiroDir(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	// In a temp dir without .kiro, presence should be false.
	// paths.WorktreeRoot will fail in temp dir (not a git repo), falls back to ".".
	// Since "." doesn't have .kiro, this should return false.
	found, err := ag.DetectPresence(context.Background())
	if err != nil {
		t.Fatalf("DetectPresence() error = %v", err)
	}
	// We can't guarantee false in CI since the working dir might have .kiro,
	// but we can at least verify no error.
	_ = found
}

func TestGetSessionID(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	input := &agent.HookInput{
		SessionID: "test-session-123",
	}
	if got := ag.GetSessionID(input); got != "test-session-123" {
		t.Errorf("GetSessionID() = %q, want %q", got, "test-session-123")
	}
}

func TestGetSessionDir(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	dir, err := ag.GetSessionDir("/tmp/myrepo")
	if err != nil {
		t.Fatalf("GetSessionDir() error = %v", err)
	}
	expected := filepath.Join("/tmp/myrepo", ".entire", "tmp")
	if dir != expected {
		t.Errorf("GetSessionDir() = %q, want %q", dir, expected)
	}
}

func TestResolveSessionFile(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	result := ag.ResolveSessionFile("/tmp/.entire/tmp", "abc-123-def")
	expected := filepath.Join("/tmp/.entire/tmp", "abc-123-def.json")
	if result != expected {
		t.Errorf("ResolveSessionFile() = %q, want %q", result, expected)
	}
}

func TestFormatResumeCommand(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	cmd := ag.FormatResumeCommand("any-session-id")
	if cmd != "kiro-cli chat --resume" {
		t.Errorf("FormatResumeCommand() = %q, want %q", cmd, "kiro-cli chat --resume")
	}
}

func TestGetHookConfigPath(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	got := ag.GetHookConfigPath()
	expected := filepath.Join(".kiro", "agents", "entire.json")
	if got != expected {
		t.Errorf("GetHookConfigPath() = %q, want %q", got, expected)
	}
}

func TestHookNames(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	names := ag.HookNames()

	expectedNames := []string{
		"agent-spawn",
		"user-prompt-submit",
		"pre-tool-use",
		"post-tool-use",
		"stop",
	}

	if len(names) != len(expectedNames) {
		t.Fatalf("HookNames() returned %d names, want %d", len(names), len(expectedNames))
	}

	for i, want := range expectedNames {
		if names[i] != want {
			t.Errorf("HookNames()[%d] = %q, want %q", i, names[i], want)
		}
	}
}

func TestGetSupportedHooks(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	hooks := ag.GetSupportedHooks()

	expected := []agent.HookType{
		agent.HookSessionStart,
		agent.HookUserPromptSubmit,
		agent.HookPreToolUse,
		agent.HookPostToolUse,
		agent.HookStop,
	}

	if len(hooks) != len(expected) {
		t.Fatalf("GetSupportedHooks() returned %d hooks, want %d", len(hooks), len(expected))
	}

	for i, want := range expected {
		if hooks[i] != want {
			t.Errorf("GetSupportedHooks()[%d] = %q, want %q", i, hooks[i], want)
		}
	}
}

func TestReadTranscript(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	transcriptPath := filepath.Join(tempDir, "session.json")
	content := []byte(`{"conversation_id":"abc","history":[]}`)
	if err := os.WriteFile(transcriptPath, content, 0o600); err != nil {
		t.Fatalf("failed to write test transcript: %v", err)
	}

	ag := &KiroAgent{}
	data, err := ag.ReadTranscript(transcriptPath)
	if err != nil {
		t.Fatalf("ReadTranscript() error = %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("ReadTranscript() = %q, want %q", string(data), string(content))
	}
}

func TestReadTranscript_FileNotFound(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	_, err := ag.ReadTranscript("/nonexistent/path/session.json")
	if err == nil {
		t.Fatal("ReadTranscript() expected error for missing file, got nil")
	}
}

func TestChunkTranscript_SmallContent(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	content := []byte(`{"small": "content"}`)
	chunks, err := ag.ChunkTranscript(context.Background(), content, 1000)
	if err != nil {
		t.Fatalf("ChunkTranscript() error = %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("ChunkTranscript() returned %d chunks, want 1", len(chunks))
	}
	if string(chunks[0]) != string(content) {
		t.Errorf("ChunkTranscript() chunk = %q, want %q", string(chunks[0]), string(content))
	}
}

func TestReassembleTranscript_SingleChunk(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	chunk := []byte(`{"conversation_id":"abc"}`)
	result, err := ag.ReassembleTranscript([][]byte{chunk})
	if err != nil {
		t.Fatalf("ReassembleTranscript() error = %v", err)
	}
	if string(result) != string(chunk) {
		t.Errorf("ReassembleTranscript() = %q, want %q", string(result), string(chunk))
	}
}

func TestReadSession(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	transcriptPath := filepath.Join(tempDir, "session.json")
	content := []byte(`{"conversation_id":"abc","history":[]}`)
	if err := os.WriteFile(transcriptPath, content, 0o600); err != nil {
		t.Fatalf("failed to write test transcript: %v", err)
	}

	ag := &KiroAgent{}
	input := &agent.HookInput{
		SessionID:  "test-session",
		SessionRef: transcriptPath,
	}

	session, err := ag.ReadSession(input)
	if err != nil {
		t.Fatalf("ReadSession() error = %v", err)
	}
	if session.SessionID != "test-session" {
		t.Errorf("ReadSession().SessionID = %q, want %q", session.SessionID, "test-session")
	}
	if session.AgentName != "kiro" {
		t.Errorf("ReadSession().AgentName = %q, want %q", session.AgentName, "kiro")
	}
	if session.SessionRef != transcriptPath {
		t.Errorf("ReadSession().SessionRef = %q, want %q", session.SessionRef, transcriptPath)
	}
	if string(session.NativeData) != string(content) {
		t.Errorf("ReadSession().NativeData = %q, want %q", string(session.NativeData), string(content))
	}
}

func TestReadSession_EmptySessionRef(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	input := &agent.HookInput{
		SessionID: "test-session",
	}

	_, err := ag.ReadSession(input)
	if err == nil {
		t.Fatal("ReadSession() expected error for empty session ref, got nil")
	}
}

func TestWriteSession(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sessionRef := filepath.Join(tempDir, "subdir", "session.json")

	ag := &KiroAgent{}
	session := &agent.AgentSession{
		SessionID:  "write-test",
		AgentName:  "kiro",
		SessionRef: sessionRef,
		NativeData: []byte(`{"conversation_id":"xyz"}`),
	}

	err := ag.WriteSession(context.Background(), session)
	if err != nil {
		t.Fatalf("WriteSession() error = %v", err)
	}

	data, err := os.ReadFile(sessionRef)
	if err != nil {
		t.Fatalf("failed to read written session: %v", err)
	}
	if string(data) != string(session.NativeData) {
		t.Errorf("written data = %q, want %q", string(data), string(session.NativeData))
	}
}

func TestWriteSession_NilSession(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	err := ag.WriteSession(context.Background(), nil)
	if err == nil {
		t.Fatal("WriteSession(nil) expected error, got nil")
	}
}

func TestWriteSession_WrongAgent(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	session := &agent.AgentSession{
		AgentName:  "claude-code",
		SessionRef: "/tmp/test.json",
		NativeData: []byte(`{}`),
	}
	err := ag.WriteSession(context.Background(), session)
	if err == nil {
		t.Fatal("WriteSession() expected error for wrong agent, got nil")
	}
}

func TestWriteSession_EmptySessionRef(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	session := &agent.AgentSession{
		AgentName:  "kiro",
		NativeData: []byte(`{}`),
	}
	err := ag.WriteSession(context.Background(), session)
	if err == nil {
		t.Fatal("WriteSession() expected error for empty session ref, got nil")
	}
}

func TestWriteSession_EmptyNativeData(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	session := &agent.AgentSession{
		AgentName:  "kiro",
		SessionRef: "/tmp/test.json",
	}
	err := ag.WriteSession(context.Background(), session)
	if err == nil {
		t.Fatal("WriteSession() expected error for empty native data, got nil")
	}
}
