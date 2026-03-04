package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleTranscriptLines returns JSONL lines matching real Cursor transcript format.
func sampleTranscriptLines() []string {
	return []string{
		`{"role":"user","message":{"content":[{"type":"text","text":"<user_query>\nhello\n</user_query>"}]}}`,
		`{"role":"assistant","message":{"content":[{"type":"text","text":"Hi there!"}]}}`,
		`{"role":"user","message":{"content":[{"type":"text","text":"<user_query>\nadd 'one' to a file and commit\n</user_query>"}]}}`,
		`{"role":"assistant","message":{"content":[{"type":"text","text":"Created one.txt with one and committed."}]}}`,
	}
}

func writeSampleTranscript(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "transcript.jsonl")
	content := strings.Join(sampleTranscriptLines(), "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write sample transcript: %v", err)
	}
	return path
}

func TestResolveSessionFile_FlatLayout(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	flatFile := filepath.Join(tmpDir, "abc123.jsonl")
	if err := os.WriteFile(flatFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write flat file: %v", err)
	}
	result := resolveSessionFile(tmpDir, "abc123")
	if result != flatFile {
		t.Errorf("resolveSessionFile() flat = %q, want %q", result, flatFile)
	}
}

func TestResolveSessionFile_NeitherExists(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	result := resolveSessionFile(tmpDir, "abc123")
	expected := filepath.Join(tmpDir, "abc123.jsonl")
	if result != expected {
		t.Errorf("resolveSessionFile() neither = %q, want %q", result, expected)
	}
}

func TestResolveSessionFile_NestedLayout(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "abc123")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	nestedFile := filepath.Join(nestedDir, "abc123.jsonl")
	if err := os.WriteFile(nestedFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	result := resolveSessionFile(tmpDir, "abc123")
	if result != nestedFile {
		t.Errorf("resolveSessionFile() nested = %q, want %q", result, nestedFile)
	}
}

func TestResolveSessionFile_NestedDirOnly(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "abc123")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	result := resolveSessionFile(tmpDir, "abc123")
	expected := filepath.Join(nestedDir, "abc123.jsonl")
	if result != expected {
		t.Errorf("resolveSessionFile() nested dir only = %q, want %q", result, expected)
	}
}

func TestResolveSessionFile_PrefersNested(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	flatFile := filepath.Join(tmpDir, "abc123.jsonl")
	if err := os.WriteFile(flatFile, []byte("flat"), 0o644); err != nil {
		t.Fatalf("failed to write flat file: %v", err)
	}

	nestedDir := filepath.Join(tmpDir, "abc123")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	nestedFile := filepath.Join(nestedDir, "abc123.jsonl")
	if err := os.WriteFile(nestedFile, []byte("nested"), 0o644); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	result := resolveSessionFile(tmpDir, "abc123")
	if result != nestedFile {
		t.Errorf("resolveSessionFile() should prefer nested = %q, got %q", nestedFile, result)
	}
}

func TestGetSessionDir_EnvOverride(t *testing.T) {
	t.Setenv("ENTIRE_TEST_CURSOR_PROJECT_DIR", "/test/override")

	dir, err := getSessionDir("/some/repo")
	if err != nil {
		t.Fatalf("getSessionDir() error = %v", err)
	}
	if dir != "/test/override" {
		t.Errorf("getSessionDir() = %q, want /test/override", dir)
	}
}

func TestGetSessionDir_DefaultPath(t *testing.T) {
	t.Setenv("ENTIRE_TEST_CURSOR_PROJECT_DIR", "")

	dir, err := getSessionDir("/some/repo")
	if err != nil {
		t.Fatalf("getSessionDir() error = %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("getSessionDir() should return absolute path, got %q", dir)
	}
	if !strings.Contains(dir, ".cursor") {
		t.Errorf("getSessionDir() = %q, expected path containing .cursor", dir)
	}
	if !strings.HasSuffix(dir, "agent-transcripts") {
		t.Errorf("getSessionDir() = %q, expected path ending with agent-transcripts", dir)
	}
}
