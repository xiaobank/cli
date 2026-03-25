package improve

import (
	"errors"
	"os"
	"path/filepath"
)

// knownContextFiles lists all known context file types and their relative paths.
var knownContextFiles = []struct {
	fileType ContextFileType
	relPath  string
}{
	{ContextFileCLAUDEMD, "CLAUDE.md"},
	{ContextFileAGENTSMD, "AGENTS.md"},
	{ContextFileCursorRules, ".cursorrules"},
	{ContextFileGemini, ".gemini/settings.json"},
}

// DetectContextFiles scans a project root for known context files.
// Returns entries for all known types, with Exists=false for missing files.
func DetectContextFiles(root string) []ContextFile {
	results := make([]ContextFile, 0, len(knownContextFiles))

	for _, known := range knownContextFiles {
		absPath := filepath.Join(root, known.relPath)
		cf := ContextFile{
			Type: known.fileType,
			Path: absPath,
		}

		data, err := os.ReadFile(absPath) //nolint:gosec // path is constructed from trusted root + known relative paths
		if err == nil {
			cf.Exists = true
			cf.Content = string(data)
			cf.SizeBytes = len(data)
		} else if !errors.Is(err, os.ErrNotExist) {
			// File exists but cannot be read — mark as existing without content.
			// Permissions errors, symlink issues, etc. fall here.
			cf.Exists = true
		}

		results = append(results, cf)
	}

	return results
}
