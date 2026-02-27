package kiro

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// Compile-time interface checks
var (
	_ agent.TranscriptAnalyzer = (*KiroAgent)(nil)
	_ agent.TranscriptPreparer = (*KiroAgent)(nil)
	_ agent.TokenCalculator    = (*KiroAgent)(nil)
)

func TestGetTranscriptPosition(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}
	path := writeTestConversation(t, testConversationJSON)

	pos, err := ag.GetTranscriptPosition(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 4 {
		t.Errorf("expected position 4, got %d", pos)
	}
}

func TestGetTranscriptPosition_NonexistentFile(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	pos, err := ag.GetTranscriptPosition("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 0 {
		t.Errorf("expected position 0 for nonexistent file, got %d", pos)
	}
}

func TestExtractModifiedFilesFromOffset(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}
	path := writeTestConversation(t, testConversationJSON)

	// From offset 0 — should get both main.go and util.go
	files, pos, err := ag.ExtractModifiedFilesFromOffset(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 4 {
		t.Errorf("expected position 4, got %d", pos)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestExtractModifiedFilesFromOffset_WithOffset(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}
	path := writeTestConversation(t, testConversationJSON)

	// From offset 2 — should only get util.go (entries 3 and 4)
	files, pos, err := ag.ExtractModifiedFilesFromOffset(path, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 4 {
		t.Errorf("expected position 4, got %d", pos)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if files[0] != "util.go" {
		t.Errorf("expected 'util.go', got %q", files[0])
	}
}

func TestExtractModifiedFilesFromOffset_NonexistentFile(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	files, pos, err := ag.ExtractModifiedFilesFromOffset("/nonexistent/path.json", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 0 {
		t.Errorf("expected position 0, got %d", pos)
	}
	if files != nil {
		t.Errorf("expected nil files, got %v", files)
	}
}

func TestExtractPrompts(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}
	path := writeTestConversation(t, testConversationJSON)

	// From offset 0 — both prompts
	prompts, err := ag.ExtractPrompts(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d: %v", len(prompts), prompts)
	}
	if prompts[0] != testPrompt1 {
		t.Errorf("expected first prompt 'Fix the bug in main.go', got %q", prompts[0])
	}
	if prompts[1] != testPrompt2 {
		t.Errorf("expected second prompt 'Also fix util.go', got %q", prompts[1])
	}
}

func TestExtractPrompts_WithOffset(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}
	path := writeTestConversation(t, testConversationJSON)

	// From offset 2 — only second prompt
	prompts, err := ag.ExtractPrompts(path, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt from offset 2, got %d", len(prompts))
	}
	if prompts[0] != testPrompt2 {
		t.Errorf("expected 'Also fix util.go', got %q", prompts[0])
	}
}

func TestExtractPrompts_NonexistentFile(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	prompts, err := ag.ExtractPrompts("/nonexistent/path.json", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompts != nil {
		t.Errorf("expected nil for nonexistent file, got %v", prompts)
	}
}

func TestExtractSummary(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}
	path := writeTestConversation(t, testConversationJSON)

	summary, err := ag.ExtractSummary(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "Done fixing util.go." {
		t.Errorf("expected summary 'Done fixing util.go.', got %q", summary)
	}
}

func TestExtractSummary_EmptyConversation(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	emptyConv := `{"conversation_id":"empty","history":[]}`
	path := writeTestConversation(t, emptyConv)

	summary, err := ag.ExtractSummary(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "" {
		t.Errorf("expected empty summary, got %q", summary)
	}
}

func TestExtractSummary_NonexistentFile(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	summary, err := ag.ExtractSummary("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "" {
		t.Errorf("expected empty summary for nonexistent file, got %q", summary)
	}
}

func TestCalculateTokenUsage(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	usage, err := ag.CalculateTokenUsage([]byte(testConversationWithMetadataJSON), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if usage.InputTokens != 350 {
		t.Errorf("expected 350 input tokens, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 180 {
		t.Errorf("expected 180 output tokens, got %d", usage.OutputTokens)
	}
	if usage.CacheReadTokens != 15 {
		t.Errorf("expected 15 cache read tokens, got %d", usage.CacheReadTokens)
	}
	if usage.CacheCreationTokens != 35 {
		t.Errorf("expected 35 cache creation tokens, got %d", usage.CacheCreationTokens)
	}
	if usage.APICallCount != 2 {
		t.Errorf("expected 2 API calls, got %d", usage.APICallCount)
	}
}

func TestCalculateTokenUsage_FromOffset(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	// From offset 3 — should only get the second request_metadata (index 5)
	usage, err := ag.CalculateTokenUsage([]byte(testConversationWithMetadataJSON), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.InputTokens != 200 {
		t.Errorf("expected 200 input tokens, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 100 {
		t.Errorf("expected 100 output tokens, got %d", usage.OutputTokens)
	}
	if usage.APICallCount != 1 {
		t.Errorf("expected 1 API call, got %d", usage.APICallCount)
	}
}

func TestCalculateTokenUsage_EmptyData(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	usage, err := ag.CalculateTokenUsage(nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage != nil {
		t.Errorf("expected nil usage for empty data, got %+v", usage)
	}
}

func TestCalculateTokenUsage_NoMetadata(t *testing.T) {
	t.Parallel()
	ag := &KiroAgent{}

	// testConversationJSON has no request_metadata entries
	usage, err := ag.CalculateTokenUsage([]byte(testConversationJSON), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if usage.InputTokens != 0 {
		t.Errorf("expected 0 input tokens, got %d", usage.InputTokens)
	}
	if usage.APICallCount != 0 {
		t.Errorf("expected 0 API calls, got %d", usage.APICallCount)
	}
}

func TestExtractAllUserPrompts(t *testing.T) {
	t.Parallel()

	prompts, err := ExtractAllUserPrompts([]byte(testConversationJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d: %v", len(prompts), prompts)
	}
	if prompts[0] != testPrompt1 {
		t.Errorf("expected 'Fix the bug in main.go', got %q", prompts[0])
	}
	if prompts[1] != testPrompt2 {
		t.Errorf("expected 'Also fix util.go', got %q", prompts[1])
	}
}

func TestExtractAllUserPrompts_Empty(t *testing.T) {
	t.Parallel()

	prompts, err := ExtractAllUserPrompts(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompts != nil {
		t.Errorf("expected nil for nil data, got %v", prompts)
	}
}

func TestExtractTextFromContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parts []ContentPart
		want  string
	}{
		{
			name:  "single text part",
			parts: []ContentPart{{Type: "text", Text: "Hello"}},
			want:  "Hello",
		},
		{
			name: "multiple text parts",
			parts: []ContentPart{
				{Type: "text", Text: "Hello"},
				{Type: "text", Text: "World"},
			},
			want: "Hello\nWorld",
		},
		{
			name: "mixed parts",
			parts: []ContentPart{
				{Type: "text", Text: "Hello"},
				{Type: "tool_use", Name: "fs_write"},
				{Type: "text", Text: "Done"},
			},
			want: "Hello\nDone",
		},
		{
			name:  "no text parts",
			parts: []ContentPart{{Type: "tool_use", Name: "fs_write"}},
			want:  "",
		},
		{
			name:  "empty parts",
			parts: nil,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractTextFromContent(tt.parts)
			if got != tt.want {
				t.Errorf("extractTextFromContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractFilePathsFromInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input any
		want  []string
	}{
		{
			name:  "file_path key",
			input: map[string]any{"file_path": "main.go"},
			want:  []string{"main.go"},
		},
		{
			name:  "path key",
			input: map[string]any{"path": "util.go"},
			want:  []string{"util.go"},
		},
		{
			name:  "filePath key (camelCase)",
			input: map[string]any{"filePath": "handler.go"},
			want:  []string{"handler.go"},
		},
		{
			name:  "no recognized key",
			input: map[string]any{"content": "some code"},
			want:  nil,
		},
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name:  "non-map input",
			input: "string-input",
			want:  nil,
		},
		{
			name:  "empty string value",
			input: map[string]any{"file_path": ""},
			want:  nil,
		},
		{
			name:  "whitespace-only value",
			input: map[string]any{"file_path": "  "},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractFilePathsFromInput(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("extractFilePathsFromInput() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsFileModificationTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		want bool
	}{
		{"fs_write", "fs_write", true},
		{"str_replace", "str_replace", true},
		{"create_file", "create_file", true},
		{"write_file", "write_file", true},
		{"edit_file", "edit_file", true},
		{"read_file", "read_file", false},
		{"unknown", "unknown_tool", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isFileModificationTool(tt.tool)
			if got != tt.want {
				t.Errorf("isFileModificationTool(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func TestReadTranscript(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.json")
	content := []byte(testConversationJSON)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	data, err := ag.ReadTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Error("read data does not match written data")
	}
}

func TestReadTranscript_NonexistentFile(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	_, err := ag.ReadTranscript("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
