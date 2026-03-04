package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFromBytes_CursorFormat(t *testing.T) {
	t.Parallel()

	content := strings.Join(sampleTranscriptLines(), "\n") + "\n"
	lines, err := parseFromBytes([]byte(content))
	if err != nil {
		t.Fatalf("parseFromBytes() error = %v", err)
	}
	if len(lines) != 4 {
		t.Fatalf("parseFromBytes() returned %d lines, want 4", len(lines))
	}
	if lines[0].Type != typeUser {
		t.Errorf("line[0].Type = %q, want %q", lines[0].Type, typeUser)
	}
	if lines[1].Type != typeAssistant {
		t.Errorf("line[1].Type = %q, want %q", lines[1].Type, typeAssistant)
	}
}

func TestParseFromFileAtLine(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := writeSampleTranscript(t, tmpDir)

	lines, err := parseFromFileAtLine(path, 2)
	if err != nil {
		t.Fatalf("parseFromFileAtLine() error = %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("parseFromFileAtLine(offset=2) returned %d lines, want 2", len(lines))
	}
	if lines[0].Type != typeUser {
		t.Errorf("line[0].Type = %q, want %q", lines[0].Type, typeUser)
	}
}

func TestExtractUserContent_StringContent(t *testing.T) {
	t.Parallel()

	msg := json.RawMessage(`{"content":"hello world"}`)
	result := extractUserContent(msg)
	if result != "hello world" {
		t.Errorf("extractUserContent() = %q, want %q", result, "hello world")
	}
}

func TestExtractUserContent_ArrayContent(t *testing.T) {
	t.Parallel()

	msg := json.RawMessage(`{"content":[{"type":"text","text":"<user_query>\nhello\n</user_query>"}]}`)
	result := extractUserContent(msg)
	if result != "hello" {
		t.Errorf("extractUserContent() = %q, want %q", result, "hello")
	}
}

func TestExtractPrompts_FromTranscript(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := writeSampleTranscript(t, tmpDir)

	lines, err := parseFromFileAtLine(path, 0)
	if err != nil {
		t.Fatalf("parseFromFileAtLine() error = %v", err)
	}

	var prompts []string
	for i := range lines {
		if lines[i].Type != typeUser {
			continue
		}
		content := extractUserContent(lines[i].Message)
		if content != "" {
			prompts = append(prompts, stripIDEContextTags(content))
		}
	}

	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(prompts))
	}
	if prompts[0] != "hello" {
		t.Errorf("prompts[0] = %q, want %q", prompts[0], "hello")
	}
	if prompts[1] != "add 'one' to a file and commit" {
		t.Errorf("prompts[1] = %q, want %q", prompts[1], "add 'one' to a file and commit")
	}
}

func TestExtractSummary_FromTranscript(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	content := strings.Join(sampleTranscriptLines(), "\n") + "\n"
	path := filepath.Join(tmpDir, "transcript.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	lines, parseErr := parseFromBytes(data)
	if parseErr != nil {
		t.Fatal(parseErr)
	}

	var summary string
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Type != typeAssistant {
			continue
		}
		var msg assistantMessage
		if err := json.Unmarshal(lines[i].Message, &msg); err != nil {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == contentTypeText && block.Text != "" {
				summary = block.Text
				break
			}
		}
		if summary != "" {
			break
		}
	}

	if summary != "Created one.txt with one and committed." {
		t.Errorf("summary = %q, want %q", summary, "Created one.txt with one and committed.")
	}
}

func TestGetTranscriptPosition_LineCount(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := writeSampleTranscript(t, tmpDir)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := parseFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 4 {
		t.Errorf("line count = %d, want 4", len(lines))
	}
}
