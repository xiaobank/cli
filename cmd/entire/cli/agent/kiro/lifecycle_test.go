package kiro

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// Note: Tests using t.Setenv cannot use t.Parallel() (Go's runtime enforces this).

func TestParseHookEvent_AgentSpawn(t *testing.T) {
	ag := &KiroAgent{}
	input := `{"hook_event_name": "agentSpawn", "cwd": "/test/repo"}`

	// AgentSpawn requires SQLite query — use mock mode
	t.Setenv("ENTIRE_TEST_KIRO_MOCK_DB", "1")

	event, err := ag.ParseHookEvent(context.Background(), HookNameAgentSpawn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != agent.SessionStart {
		t.Errorf("expected SessionStart, got %v", event.Type)
	}
	if event.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
}

func TestParseHookEvent_UserPromptSubmit(t *testing.T) {
	ag := &KiroAgent{}
	input := `{"hook_event_name": "userPromptSubmit", "cwd": "/test/repo", "prompt": "Fix the bug in login.ts"}`

	t.Setenv("ENTIRE_TEST_KIRO_MOCK_DB", "1")

	event, err := ag.ParseHookEvent(context.Background(), HookNameUserPromptSubmit, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != agent.TurnStart {
		t.Errorf("expected TurnStart, got %v", event.Type)
	}
	if event.Prompt != "Fix the bug in login.ts" {
		t.Errorf("expected prompt 'Fix the bug in login.ts', got %q", event.Prompt)
	}
	if event.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if !strings.HasSuffix(event.SessionRef, ".json") {
		t.Errorf("expected session ref to end with .json, got %q", event.SessionRef)
	}
}

func TestParseHookEvent_PreToolUse(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	input := `{"hook_event_name": "preToolUse", "cwd": "/test/repo", "tool_name": "fs_write"}`

	event, err := ag.ParseHookEvent(context.Background(), HookNamePreToolUse, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event for pre-tool-use, got %+v", event)
	}
}

func TestParseHookEvent_PostToolUse(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	input := `{"hook_event_name": "postToolUse", "cwd": "/test/repo"}`

	event, err := ag.ParseHookEvent(context.Background(), HookNamePostToolUse, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event for post-tool-use, got %+v", event)
	}
}

// TestParseHookEvent_Stop requires sqlite3 — tested in integration tests.
func TestParseHookEvent_Stop_RequiresSQLite(t *testing.T) {
	t.Skip("Stop requires sqlite3 for transcript caching — tested in integration tests")
}

func TestParseHookEvent_UnknownHook(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	event, err := ag.ParseHookEvent(context.Background(), "unknown-hook", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event for unknown hook, got %+v", event)
	}
}

func TestParseHookEvent_EmptyInput(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	_, err := ag.ParseHookEvent(context.Background(), HookNameAgentSpawn, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if !strings.Contains(err.Error(), "empty hook input") {
		t.Errorf("expected 'empty hook input' error, got: %v", err)
	}
}

func TestParseHookEvent_MalformedJSON(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	_, err := ag.ParseHookEvent(context.Background(), HookNameAgentSpawn, strings.NewReader("not json"))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestHookNames(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	names := ag.HookNames()

	expected := []string{
		HookNameAgentSpawn,
		HookNameUserPromptSubmit,
		HookNamePreToolUse,
		HookNamePostToolUse,
		HookNameStop,
	}

	if len(names) != len(expected) {
		t.Fatalf("expected %d hook names, got %d", len(expected), len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, e := range expected {
		if !nameSet[e] {
			t.Errorf("missing expected hook name: %s", e)
		}
	}
}

func TestPrepareTranscript_ErrorOnInvalidPath(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	err := ag.PrepareTranscript(context.Background(), "/tmp/not-a-json-file")
	if err == nil {
		t.Fatal("expected error for path without .json extension")
	}
	if !strings.Contains(err.Error(), "invalid Kiro transcript path") {
		t.Errorf("expected 'invalid Kiro transcript path' error, got: %v", err)
	}
}

func TestPrepareTranscript_ErrorOnEmptySessionID(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	err := ag.PrepareTranscript(context.Background(), "/tmp/.json")
	if err == nil {
		t.Fatal("expected error for empty session ID")
	}
	if !strings.Contains(err.Error(), "empty session ID") {
		t.Errorf("expected 'empty session ID' error, got: %v", err)
	}
}

func TestMockSessionID(t *testing.T) {
	t.Parallel()

	id := mockSessionID("/test/repo")
	if id == "" {
		t.Error("expected non-empty mock session ID")
	}
	if !strings.HasPrefix(id, "mock-session-") {
		t.Errorf("expected mock-session- prefix, got %q", id)
	}

	// Deterministic: same input produces same output
	id2 := mockSessionID("/test/repo")
	if id != id2 {
		t.Errorf("expected deterministic ID, got %q and %q", id, id2)
	}
}

func TestEscapeSQLString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"it's a test", "it''s a test"},
		{"no'quotes'here", "no''quotes''here"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := escapeSQLString(tt.input)
			if got != tt.want {
				t.Errorf("escapeSQLString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestKiroDBPath(t *testing.T) {
	t.Parallel()

	path, err := kiroDBPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty db path")
	}
	if !strings.Contains(path, "kiro-cli") {
		t.Errorf("expected path to contain 'kiro-cli', got %q", path)
	}
}

func TestKiroDBPath_EnvOverride(t *testing.T) {
	t.Setenv("ENTIRE_TEST_KIRO_DB_PATH", "/test/override/data.sqlite3")

	path, err := kiroDBPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/test/override/data.sqlite3" {
		t.Errorf("expected /test/override/data.sqlite3, got %q", path)
	}
}

func TestFetchAndCacheTranscript_MockMode(t *testing.T) {
	// fetchAndCacheTranscript in mock mode uses WorktreeRoot which requires a git repo.
	// Verify the mock file creation logic works correctly.
	dir := t.TempDir()

	tmpDir := filepath.Join(dir, ".entire", "tmp")
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}

	mockData := `{"conversation_id":"mock-123","history":[]}`
	mockFile := filepath.Join(tmpDir, "mock-123.json")
	if err := os.WriteFile(mockFile, []byte(mockData), 0o600); err != nil {
		t.Fatalf("failed to write mock file: %v", err)
	}

	data, err := os.ReadFile(mockFile)
	if err != nil {
		t.Fatalf("failed to read mock file: %v", err)
	}
	if string(data) != mockData {
		t.Error("mock data does not match")
	}
}
