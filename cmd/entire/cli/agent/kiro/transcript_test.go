package kiro

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testKiroTranscript is a realistic 4-entry Kiro transcript:
//
//	[0] Prompt: "Create a hello.go file" → Response: "I'll create..."
//	[1] Prompt: "Now add a test"         → ToolUse: fs_write /repo/hello.go
//	[2] ToolUseResults                   → ToolUse: fs_write /repo/hello_test.go
//	[3] ToolUseResults                   → Response: "Done! I created both files."
const testKiroTranscript = `{
  "conversation_id": "test-conv-123",
  "history": [
    {
      "user": {"content": {"Prompt": {"prompt": "Create a hello.go file"}}, "timestamp": "2026-01-01T00:00:00Z"},
      "assistant": {"Response": {"message_id": "msg-1", "content": "I'll create that file for you."}}
    },
    {
      "user": {"content": {"Prompt": {"prompt": "Now add a test"}}, "timestamp": "2026-01-01T00:01:00Z"},
      "assistant": {"ToolUse": {"message_id": "msg-2", "tool_uses": [
        {"id": "tu-1", "name": "fs_write", "args": {"path": "/repo/hello.go", "content": "package main"}}
      ]}}
    },
    {
      "user": {"content": {"ToolUseResults": {"tool_use_results": [{"id": "tu-1", "result": "ok"}]}}},
      "assistant": {"ToolUse": {"message_id": "msg-3", "tool_uses": [
        {"id": "tu-2", "name": "fs_write", "args": {"path": "/repo/hello_test.go", "content": "package main"}}
      ]}}
    },
    {
      "user": {"content": {"ToolUseResults": {"tool_use_results": [{"id": "tu-2", "result": "ok"}]}}},
      "assistant": {"Response": {"message_id": "msg-4", "content": "Done! I created both files."}}
    }
  ]
}`

// --- parseTranscript ---

func TestParseTranscript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       []byte
		wantEntries int
		wantConvID  string
		wantErr     bool
	}{
		{
			name:        "valid transcript",
			input:       []byte(testKiroTranscript),
			wantEntries: 4,
			wantConvID:  "test-conv-123",
		},
		{
			name:        "empty history",
			input:       []byte(`{"conversation_id":"abc","history":[]}`),
			wantEntries: 0,
			wantConvID:  "abc",
		},
		{
			name:        "placeholder {}",
			input:       []byte(`{}`),
			wantEntries: 0,
			wantConvID:  "",
		},
		{
			name:        "empty bytes",
			input:       []byte{},
			wantEntries: 0,
			wantConvID:  "",
		},
		{
			name:    "invalid JSON",
			input:   []byte(`{not json`),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTranscript(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ConversationID != tc.wantConvID {
				t.Errorf("ConversationID = %q, want %q", got.ConversationID, tc.wantConvID)
			}
			if len(got.History) != tc.wantEntries {
				t.Errorf("len(History) = %d, want %d", len(got.History), tc.wantEntries)
			}
		})
	}
}

// --- extractUserPrompt ---

func TestExtractUserPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "Prompt variant",
			raw:  `{"Prompt": {"prompt": "hello world"}}`,
			want: "hello world",
		},
		{
			name: "ToolUseResults variant",
			raw:  `{"ToolUseResults": {"tool_use_results": []}}`,
			want: "",
		},
		{
			name: "empty content",
			raw:  `{}`,
			want: "",
		},
		{
			name: "null content",
			raw:  "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractUserPrompt(json.RawMessage(tc.raw))
			if got != tc.want {
				t.Errorf("extractUserPrompt() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- extractModifiedFilesFromHistory ---

func TestExtractModifiedFilesFromHistory(t *testing.T) {
	t.Parallel()

	transcript, err := parseTranscript([]byte(testKiroTranscript))
	if err != nil {
		t.Fatalf("failed to parse test transcript: %v", err)
	}

	tests := []struct {
		name      string
		entries   []kiroHistoryEntry
		wantFiles []string
	}{
		{
			name:      "all entries - finds both fs_write files",
			entries:   transcript.History,
			wantFiles: []string{"/repo/hello.go", "/repo/hello_test.go"},
		},
		{
			name:      "from offset 2 - only second file",
			entries:   transcript.History[2:],
			wantFiles: []string{"/repo/hello_test.go"},
		},
		{
			name:      "first entry only - no tool use",
			entries:   transcript.History[:1],
			wantFiles: nil,
		},
		{
			name:      "empty entries",
			entries:   nil,
			wantFiles: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractModifiedFilesFromHistory(tc.entries)
			if len(got) != len(tc.wantFiles) {
				t.Fatalf("got %d files %v, want %d files %v", len(got), got, len(tc.wantFiles), tc.wantFiles)
			}
			for i, want := range tc.wantFiles {
				if got[i] != want {
					t.Errorf("files[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

func TestExtractModifiedFilesFromHistory_Dedup(t *testing.T) {
	t.Parallel()

	// Two tool calls writing to the same file should only appear once.
	transcript := `{
		"conversation_id": "dedup-test",
		"history": [
			{
				"user": {"content": {"Prompt": {"prompt": "write"}}},
				"assistant": {"ToolUse": {"message_id": "m1", "tool_uses": [
					{"id": "t1", "name": "fs_write", "args": {"path": "/repo/main.go"}}
				]}}
			},
			{
				"user": {"content": {"ToolUseResults": {}}},
				"assistant": {"ToolUse": {"message_id": "m2", "tool_uses": [
					{"id": "t2", "name": "fs_edit", "args": {"path": "/repo/main.go"}}
				]}}
			}
		]
	}`

	t2, err := parseTranscript([]byte(transcript))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	files := extractModifiedFilesFromHistory(t2.History)
	if len(files) != 1 || files[0] != "/repo/main.go" {
		t.Errorf("got %v, want [/repo/main.go]", files)
	}
}

func TestExtractModifiedFilesFromHistory_NonFileTool(t *testing.T) {
	t.Parallel()

	transcript := `{
		"conversation_id": "non-file",
		"history": [{
			"user": {"content": {"Prompt": {"prompt": "search"}}},
			"assistant": {"ToolUse": {"message_id": "m1", "tool_uses": [
				{"id": "t1", "name": "shell_exec", "args": {"command": "ls"}}
			]}}
		}]
	}`

	t2, err := parseTranscript([]byte(transcript))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	files := extractModifiedFilesFromHistory(t2.History)
	if len(files) != 0 {
		t.Errorf("got %v, want empty", files)
	}
}

// --- extractFilePath ---

func TestExtractFilePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args string
		want string
	}{
		{
			name: "path key",
			args: `{"path": "/repo/file.go", "content": "..."}`,
			want: "/repo/file.go",
		},
		{
			name: "file_path key",
			args: `{"file_path": "/repo/other.go"}`,
			want: "/repo/other.go",
		},
		{
			name: "filename key",
			args: `{"filename": "/repo/third.go"}`,
			want: "/repo/third.go",
		},
		{
			name: "path takes priority over file_path",
			args: `{"path": "/first", "file_path": "/second"}`,
			want: "/first",
		},
		{
			name: "empty args",
			args: `{}`,
			want: "",
		},
		{
			name: "null args",
			args: "",
			want: "",
		},
		{
			name: "no path keys",
			args: `{"content": "some text"}`,
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractFilePath(json.RawMessage(tc.args))
			if got != tc.want {
				t.Errorf("extractFilePath() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- extractLastAssistantResponse ---

func TestExtractLastAssistantResponse(t *testing.T) {
	t.Parallel()

	transcript, err := parseTranscript([]byte(testKiroTranscript))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	tests := []struct {
		name    string
		entries []kiroHistoryEntry
		want    string
	}{
		{
			name:    "full transcript - last Response",
			entries: transcript.History,
			want:    "Done! I created both files.",
		},
		{
			name:    "only first two entries - first Response",
			entries: transcript.History[:2],
			want:    "I'll create that file for you.",
		},
		{
			name:    "single ToolUse entry - no Response",
			entries: transcript.History[2:3],
			want:    "",
		},
		{
			name:    "empty entries",
			entries: nil,
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractLastAssistantResponse(tc.entries)
			if got != tc.want {
				t.Errorf("extractLastAssistantResponse() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- GetTranscriptPosition ---

func TestGetTranscriptPosition(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	t.Run("empty path", func(t *testing.T) {
		t.Parallel()
		pos, err := ag.GetTranscriptPosition("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 0 {
			t.Errorf("got %d, want 0", pos)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		pos, err := ag.GetTranscriptPosition("/nonexistent/file.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 0 {
			t.Errorf("got %d, want 0", pos)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, "")
		pos, err := ag.GetTranscriptPosition(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 0 {
			t.Errorf("got %d, want 0", pos)
		}
	})

	t.Run("placeholder {}", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, "{}")
		pos, err := ag.GetTranscriptPosition(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 0 {
			t.Errorf("got %d, want 0", pos)
		}
	})

	t.Run("normal transcript", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, testKiroTranscript)
		pos, err := ag.GetTranscriptPosition(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 4 {
			t.Errorf("got %d, want 4", pos)
		}
	})
}

// --- ExtractModifiedFilesFromOffset ---

func TestExtractModifiedFilesFromOffset(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	t.Run("offset 0 - all files", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, testKiroTranscript)
		files, pos, err := ag.ExtractModifiedFilesFromOffset(path, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 4 {
			t.Errorf("position = %d, want 4", pos)
		}
		if len(files) != 2 {
			t.Fatalf("got %d files, want 2: %v", len(files), files)
		}
	})

	t.Run("offset 2 - only second file", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, testKiroTranscript)
		files, pos, err := ag.ExtractModifiedFilesFromOffset(path, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 4 {
			t.Errorf("position = %d, want 4", pos)
		}
		if len(files) != 1 || files[0] != "/repo/hello_test.go" {
			t.Errorf("got %v, want [/repo/hello_test.go]", files)
		}
	})

	t.Run("offset >= len - no files", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, testKiroTranscript)
		files, pos, err := ag.ExtractModifiedFilesFromOffset(path, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 4 {
			t.Errorf("position = %d, want 4", pos)
		}
		if len(files) != 0 {
			t.Errorf("got %v, want empty", files)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		files, pos, err := ag.ExtractModifiedFilesFromOffset("/nonexistent.json", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 0 || len(files) != 0 {
			t.Errorf("expected zero pos and empty files, got pos=%d files=%v", pos, files)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		t.Parallel()
		files, pos, err := ag.ExtractModifiedFilesFromOffset("", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pos != 0 || len(files) != 0 {
			t.Errorf("expected zero pos and empty files, got pos=%d files=%v", pos, files)
		}
	})
}

// --- ExtractPrompts ---

func TestExtractPrompts(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	t.Run("all prompts from offset 0", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, testKiroTranscript)
		prompts, err := ag.ExtractPrompts(path, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Entries 0 and 1 have Prompt content; entries 2 and 3 have ToolUseResults.
		if len(prompts) != 2 {
			t.Fatalf("got %d prompts, want 2: %v", len(prompts), prompts)
		}
		if prompts[0] != "Create a hello.go file" {
			t.Errorf("prompts[0] = %q, want %q", prompts[0], "Create a hello.go file")
		}
		if prompts[1] != "Now add a test" {
			t.Errorf("prompts[1] = %q, want %q", prompts[1], "Now add a test")
		}
	})

	t.Run("with offset skips first prompt", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, testKiroTranscript)
		prompts, err := ag.ExtractPrompts(path, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(prompts) != 1 || prompts[0] != "Now add a test" {
			t.Errorf("got %v, want [Now add a test]", prompts)
		}
	})

	t.Run("offset beyond all prompts", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, testKiroTranscript)
		prompts, err := ag.ExtractPrompts(path, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Entries 2 and 3 are ToolUseResults, no prompts.
		if len(prompts) != 0 {
			t.Errorf("got %v, want empty", prompts)
		}
	})
}

// --- ExtractSummary ---

func TestExtractSummary(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	t.Run("last Response from full transcript", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, testKiroTranscript)
		summary, err := ag.ExtractSummary(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary != "Done! I created both files." {
			t.Errorf("summary = %q, want %q", summary, "Done! I created both files.")
		}
	})

	t.Run("empty transcript", func(t *testing.T) {
		t.Parallel()
		path := writeTestFile(t, `{"conversation_id":"x","history":[]}`)
		summary, err := ag.ExtractSummary(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary != "" {
			t.Errorf("summary = %q, want empty", summary)
		}
	})

	t.Run("only ToolUse entries - no summary", func(t *testing.T) {
		t.Parallel()
		onlyToolUse := `{
			"conversation_id": "tu-only",
			"history": [{
				"user": {"content": {"Prompt": {"prompt": "write"}}},
				"assistant": {"ToolUse": {"message_id": "m1", "tool_uses": []}}
			}]
		}`
		path := writeTestFile(t, onlyToolUse)
		summary, err := ag.ExtractSummary(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary != "" {
			t.Errorf("summary = %q, want empty", summary)
		}
	})
}

// --- isFileModificationTool ---

func TestIsFileModificationTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		want bool
	}{
		{"fs_write", "fs_write", true},
		{"fs_edit", "fs_edit", true},
		{"shell_exec", "shell_exec", false},
		{"fs_read", "fs_read", false},
		{"empty", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isFileModificationTool(tc.tool); got != tc.want {
				t.Errorf("isFileModificationTool(%q) = %v, want %v", tc.tool, got, tc.want)
			}
		})
	}
}

// --- IDE format transcript tests ---

// testKiroIDETranscript is a realistic Kiro IDE transcript with sequential
// user/assistant messages in Anthropic API format.
const testKiroIDETranscript = `{
  "history": [
    {
      "message": {
        "role": "user",
        "content": [{"type": "text", "text": "Create a hello world in python"}]
      }
    },
    {
      "message": {
        "role": "assistant",
        "content": "I'll create a hello world Python script for you."
      }
    },
    {
      "message": {
        "role": "user",
        "content": [{"type": "text", "text": "Now add a test file"}]
      }
    },
    {
      "message": {
        "role": "assistant",
        "content": "Done! I've created both hello.py and test_hello.py."
      }
    }
  ]
}`

func TestParseTranscript_IDEFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       []byte
		wantEntries int
		wantErr     bool
	}{
		{
			name:        "valid IDE transcript",
			input:       []byte(testKiroIDETranscript),
			wantEntries: 2, // 4 sequential messages → 2 paired entries
		},
		{
			name: "IDE transcript with unpaired trailing user message",
			input: []byte(`{
				"history": [
					{"message": {"role": "user", "content": [{"type": "text", "text": "hello"}]}},
					{"message": {"role": "assistant", "content": "hi"}},
					{"message": {"role": "user", "content": [{"type": "text", "text": "goodbye"}]}}
				]
			}`),
			wantEntries: 2, // 2 user + 1 assistant → 2 entries (second user unpaired)
		},
		{
			name: "IDE transcript with plain string user content",
			input: []byte(`{
				"history": [
					{"message": {"role": "user", "content": "plain text prompt"}},
					{"message": {"role": "assistant", "content": "response"}}
				]
			}`),
			wantEntries: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTranscript(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.History) != tc.wantEntries {
				t.Errorf("len(History) = %d, want %d", len(got.History), tc.wantEntries)
			}
		})
	}
}

func TestExtractUserPrompt_IDEFormat(t *testing.T) {
	t.Parallel()

	// Parse IDE transcript and verify prompts are extracted correctly.
	transcript, err := parseTranscript([]byte(testKiroIDETranscript))
	if err != nil {
		t.Fatalf("failed to parse IDE transcript: %v", err)
	}

	if len(transcript.History) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(transcript.History))
	}

	// First entry should have the first prompt.
	prompt := extractUserPrompt(transcript.History[0].User.Content)
	if prompt != "Create a hello world in python" {
		t.Errorf("prompt[0] = %q, want %q", prompt, "Create a hello world in python")
	}

	// Second entry should have the second prompt.
	prompt = extractUserPrompt(transcript.History[1].User.Content)
	if prompt != "Now add a test file" {
		t.Errorf("prompt[1] = %q, want %q", prompt, "Now add a test file")
	}
}

func TestExtractLastAssistantResponse_IDEFormat(t *testing.T) {
	t.Parallel()

	transcript, err := parseTranscript([]byte(testKiroIDETranscript))
	if err != nil {
		t.Fatalf("failed to parse IDE transcript: %v", err)
	}

	summary := extractLastAssistantResponse(transcript.History)
	if summary != "Done! I've created both hello.py and test_hello.py." {
		t.Errorf("summary = %q, want %q", summary, "Done! I've created both hello.py and test_hello.py.")
	}
}

func TestExtractPrompts_IDEFormat(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	path := writeTestFile(t, testKiroIDETranscript)

	prompts, err := ag.ExtractPrompts(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("got %d prompts, want 2: %v", len(prompts), prompts)
	}
	if prompts[0] != "Create a hello world in python" {
		t.Errorf("prompts[0] = %q, want %q", prompts[0], "Create a hello world in python")
	}
	if prompts[1] != "Now add a test file" {
		t.Errorf("prompts[1] = %q, want %q", prompts[1], "Now add a test file")
	}
}

func TestExtractSummary_IDEFormat(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	path := writeTestFile(t, testKiroIDETranscript)

	summary, err := ag.ExtractSummary(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "Done! I've created both hello.py and test_hello.py." {
		t.Errorf("summary = %q, want %q", summary, "Done! I've created both hello.py and test_hello.py.")
	}
}

func TestGetTranscriptPosition_IDEFormat(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	path := writeTestFile(t, testKiroIDETranscript)

	pos, err := ag.GetTranscriptPosition(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4 sequential messages → 2 paired entries
	if pos != 2 {
		t.Errorf("got %d, want 2", pos)
	}
}

// --- IDE transcript discovery tests ---

func TestIDEWorkspaceSessionsDir(t *testing.T) {
	t.Parallel()

	dir, err := ideWorkspaceSessionsDir("/Users/alisha/Projects/test-repos/kiro-ide")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the path contains the base64-encoded CWD.
	if !strings.Contains(dir, "workspace-sessions") {
		t.Errorf("path should contain workspace-sessions: %s", dir)
	}

	// The base64 of the path should be in the directory name.
	encoded := "L1VzZXJzL2FsaXNoYS9Qcm9qZWN0cy90ZXN0LXJlcG9zL2tpcm8taWRl"
	if !strings.HasSuffix(dir, encoded) {
		t.Errorf("path should end with base64-encoded cwd %q, got %q", encoded, dir)
	}
}

func TestEnsureIDETranscript(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}

	// Set up a fake IDE workspace sessions directory.
	tmpDir := t.TempDir()
	cwd := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(cwd, 0o750); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	// We can't easily test the real IDE path, but we can test the error case.
	_, err := ag.ensureIDETranscript(context.Background(), cwd, "test-session")
	if err == nil {
		t.Error("expected error for missing IDE sessions directory, got nil")
	}
}

// --- convertIDETranscript ---

func TestConvertIDETranscript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantEntries int
		wantPrompts []string
		wantSummary string
	}{
		{
			name:        "normal paired messages",
			input:       testKiroIDETranscript,
			wantEntries: 2,
			wantPrompts: []string{"Create a hello world in python", "Now add a test file"},
			wantSummary: "Done! I've created both hello.py and test_hello.py.",
		},
		{
			name: "trailing user without assistant",
			input: `{
				"history": [
					{"message": {"role": "user", "content": [{"type": "text", "text": "hello"}]}},
					{"message": {"role": "assistant", "content": "hi there"}},
					{"message": {"role": "user", "content": [{"type": "text", "text": "are you there?"}]}}
				]
			}`,
			wantEntries: 2,
			wantPrompts: []string{"hello", "are you there?"},
			wantSummary: "hi there",
		},
		{
			name: "single exchange",
			input: `{
				"history": [
					{"message": {"role": "user", "content": "just a string"}},
					{"message": {"role": "assistant", "content": "response"}}
				]
			}`,
			wantEntries: 1,
			wantPrompts: []string{"just a string"},
			wantSummary: "response",
		},
		{
			name:        "empty history",
			input:       `{"history": []}`,
			wantEntries: 0,
			wantPrompts: nil,
			wantSummary: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTranscript([]byte(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.History) != tc.wantEntries {
				t.Errorf("len(History) = %d, want %d", len(got.History), tc.wantEntries)
			}

			// Verify prompt extraction.
			var prompts []string
			for i := range got.History {
				if p := extractUserPrompt(got.History[i].User.Content); p != "" {
					prompts = append(prompts, p)
				}
			}
			if len(prompts) != len(tc.wantPrompts) {
				t.Errorf("got %d prompts %v, want %d %v", len(prompts), prompts, len(tc.wantPrompts), tc.wantPrompts)
			} else {
				for i, want := range tc.wantPrompts {
					if prompts[i] != want {
						t.Errorf("prompts[%d] = %q, want %q", i, prompts[i], want)
					}
				}
			}

			// Verify summary extraction.
			summary := extractLastAssistantResponse(got.History)
			if summary != tc.wantSummary {
				t.Errorf("summary = %q, want %q", summary, tc.wantSummary)
			}
		})
	}
}

// writeTestFile is a helper that creates a temporary transcript file.
func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	return path
}
