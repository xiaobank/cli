package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestChunkJSONL_EmptyContent(t *testing.T) {
	t.Parallel()
	chunks, err := chunkJSONL([]byte{}, 100)
	if err != nil {
		t.Fatalf("chunkJSONL(empty) error = %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestChunkJSONL_SmallContent(t *testing.T) {
	t.Parallel()
	content := []byte(`{"line":1}` + "\n" + `{"line":2}`)
	chunks, err := chunkJSONL(content, 1000)
	if err != nil {
		t.Fatalf("chunkJSONL error = %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestChunkJSONL_ForcesMultipleChunks(t *testing.T) {
	t.Parallel()
	var lines []string
	for range 10 {
		lines = append(lines, `{"data":"`+strings.Repeat("x", 50)+`"}`)
	}
	content := []byte(strings.Join(lines, "\n"))

	chunks, err := chunkJSONL(content, 200)
	if err != nil {
		t.Fatalf("chunkJSONL error = %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}
}

func TestChunkJSONL_RoundTrip(t *testing.T) {
	t.Parallel()
	lines := []string{`{"a":1}`, `{"b":2}`, `{"c":3}`}
	original := []byte(strings.Join(lines, "\n"))

	chunks, err := chunkJSONL(original, 20)
	if err != nil {
		t.Fatalf("chunkJSONL error = %v", err)
	}

	reassembled := reassembleJSONL(chunks)
	if !bytes.Equal(original, reassembled) {
		t.Errorf("round-trip mismatch:\n  original:     %q\n  reassembled:  %q", original, reassembled)
	}
}

func TestStripIDEContextTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<user_query>\nhello\n</user_query>", "hello"},
		{"<ide_opened_file>content</ide_opened_file>hello", "hello"},
		{"<system-reminder>stuff</system-reminder>hello", "hello"},
		{"before <user_query>middle</user_query> after", "before middle after"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := stripIDEContextTags(tt.input)
			if result != tt.expected {
				t.Errorf("stripIDEContextTags(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizePathForCursor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/robin/project", "Users-robin-project"},
		{"/Users/robin/Developer/bingo", "Users-robin-Developer-bingo"},
		{"/tmp/test", "tmp-test"},
		{"simple", "simple"},
		{"/path/with spaces/dir", "path-with-spaces-dir"},
		{"/path.with.dots/dir", "path-with-dots-dir"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := sanitizePathForCursor(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizePathForCursor(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
