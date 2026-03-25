package improve_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/improve"
)

func TestDetectContextFiles_AllPresent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Create all known context files
	writeFile(t, root, "CLAUDE.md", "# Claude instructions\nDo the thing.")
	writeFile(t, root, "AGENTS.md", "# Agents instructions\nBe helpful.")
	writeFile(t, root, ".cursorrules", "cursor rules content")

	geminiDir := filepath.Join(root, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o755); err != nil {
		t.Fatalf("failed to create .gemini dir: %v", err)
	}
	writeFile(t, root, ".gemini/settings.json", `{"model":"gemini-pro"}`)

	files := improve.DetectContextFiles(root)

	if len(files) != 4 {
		t.Fatalf("expected 4 files, got %d", len(files))
	}

	byType := make(map[improve.ContextFileType]improve.ContextFile)
	for _, f := range files {
		byType[f.Type] = f
	}

	// Verify CLAUDE.md
	cf := byType[improve.ContextFileCLAUDEMD]
	if !cf.Exists {
		t.Error("CLAUDE.md: expected Exists=true")
	}
	if cf.Content != "# Claude instructions\nDo the thing." {
		t.Errorf("CLAUDE.md: unexpected content %q", cf.Content)
	}
	if cf.SizeBytes != len("# Claude instructions\nDo the thing.") {
		t.Errorf("CLAUDE.md: unexpected size %d", cf.SizeBytes)
	}
	if cf.Path != filepath.Join(root, "CLAUDE.md") {
		t.Errorf("CLAUDE.md: unexpected path %q", cf.Path)
	}

	// Verify AGENTS.md
	af := byType[improve.ContextFileAGENTSMD]
	if !af.Exists {
		t.Error("AGENTS.md: expected Exists=true")
	}
	if af.Content != "# Agents instructions\nBe helpful." {
		t.Errorf("AGENTS.md: unexpected content %q", af.Content)
	}

	// Verify .cursorrules
	cr := byType[improve.ContextFileCursorRules]
	if !cr.Exists {
		t.Error(".cursorrules: expected Exists=true")
	}
	if cr.Content != "cursor rules content" {
		t.Errorf(".cursorrules: unexpected content %q", cr.Content)
	}

	// Verify .gemini/settings.json
	gs := byType[improve.ContextFileGemini]
	if !gs.Exists {
		t.Error(".gemini/settings.json: expected Exists=true")
	}
	if gs.Content != `{"model":"gemini-pro"}` {
		t.Errorf(".gemini/settings.json: unexpected content %q", gs.Content)
	}
}

func TestDetectContextFiles_NonePresent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	files := improve.DetectContextFiles(root)

	if len(files) != 4 {
		t.Fatalf("expected 4 entries (all missing), got %d", len(files))
	}

	for _, f := range files {
		if f.Exists {
			t.Errorf("%s: expected Exists=false for missing file", f.Type)
		}
		if f.Content != "" {
			t.Errorf("%s: expected empty Content for missing file, got %q", f.Type, f.Content)
		}
		if f.SizeBytes != 0 {
			t.Errorf("%s: expected SizeBytes=0 for missing file, got %d", f.Type, f.SizeBytes)
		}
	}
}

func TestDetectContextFiles_PartialPresent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "CLAUDE.md", "only claude")

	files := improve.DetectContextFiles(root)

	if len(files) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(files))
	}

	byType := make(map[improve.ContextFileType]improve.ContextFile)
	for _, f := range files {
		byType[f.Type] = f
	}

	if !byType[improve.ContextFileCLAUDEMD].Exists {
		t.Error("CLAUDE.md: should exist")
	}
	if byType[improve.ContextFileAGENTSMD].Exists {
		t.Error("AGENTS.md: should not exist")
	}
	if byType[improve.ContextFileCursorRules].Exists {
		t.Error(".cursorrules: should not exist")
	}
	if byType[improve.ContextFileGemini].Exists {
		t.Error(".gemini/settings.json: should not exist")
	}
}

func TestDetectContextFiles_PathsAreAbsolute(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	files := improve.DetectContextFiles(root)

	for _, f := range files {
		if !filepath.IsAbs(f.Path) {
			t.Errorf("%s: expected absolute path, got %q", f.Type, f.Path)
		}
	}
}

func TestDetectContextFiles_EmptyFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "CLAUDE.md", "")

	files := improve.DetectContextFiles(root)

	byType := make(map[improve.ContextFileType]improve.ContextFile)
	for _, f := range files {
		byType[f.Type] = f
	}

	cf := byType[improve.ContextFileCLAUDEMD]
	if !cf.Exists {
		t.Error("CLAUDE.md: empty file should still exist")
	}
	if cf.SizeBytes != 0 {
		t.Errorf("CLAUDE.md: expected SizeBytes=0, got %d", cf.SizeBytes)
	}
	if cf.Content != "" {
		t.Errorf("CLAUDE.md: expected empty Content, got %q", cf.Content)
	}
}

// writeFile is a helper that creates a file with given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}
