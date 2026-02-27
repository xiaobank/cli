package kiro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Test fixture constants
const (
	testPrompt1 = "Fix the bug in main.go"
	testPrompt2 = "Also fix util.go"
)

// testConversationJSON is a Kiro conversation with 4 history entries.
var testConversationJSON = func() string {
	conv := Conversation{
		ConversationID: "test-conv-123",
		History: []HistoryEntry{
			{
				Role: "user",
				Content: []ContentPart{
					{Type: "text", Text: testPrompt1},
				},
			},
			{
				Role: "assistant",
				Content: []ContentPart{
					{Type: "text", Text: "I'll fix the bug."},
					{Type: "tool_use", Name: "fs_write", ID: "tool-1", Input: map[string]any{"file_path": "main.go", "content": "fixed"}},
				},
			},
			{
				Role: "user",
				Content: []ContentPart{
					{Type: "text", Text: testPrompt2},
				},
			},
			{
				Role: "assistant",
				Content: []ContentPart{
					{Type: "tool_use", Name: "str_replace", ID: "tool-2", Input: map[string]any{"file_path": "util.go", "old": "broken", "new": "fixed"}},
					{Type: "text", Text: "Done fixing util.go."},
				},
			},
		},
	}
	data, err := json.Marshal(conv)
	if err != nil {
		panic(err)
	}
	return string(data)
}()

// testConversationWithMetadataJSON includes request_metadata entries for token usage testing.
var testConversationWithMetadataJSON = func() string {
	conv := Conversation{
		ConversationID: "test-conv-tokens",
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
					{Type: "text", Text: "I'll fix it."},
				},
			},
			{
				Role:         "request_metadata",
				InputTokens:  150,
				OutputTokens: 80,
				CacheRead:    5,
				CacheWrite:   15,
			},
			{
				Role: "user",
				Content: []ContentPart{
					{Type: "text", Text: testPrompt2},
				},
			},
			{
				Role: "assistant",
				Content: []ContentPart{
					{Type: "text", Text: "Done."},
				},
			},
			{
				Role:         "request_metadata",
				InputTokens:  200,
				OutputTokens: 100,
				CacheRead:    10,
				CacheWrite:   20,
			},
		},
	}
	data, err := json.Marshal(conv)
	if err != nil {
		panic(err)
	}
	return string(data)
}()

// writeTestConversation writes test conversation JSON to a temp file and returns the path.
func writeTestConversation(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test conversation: %v", err)
	}
	return path
}
