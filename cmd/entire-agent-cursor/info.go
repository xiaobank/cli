package main

import (
	"os"
	"path/filepath"
)

func cmdInfo() error {
	return writeJSON(map[string]any{
		"protocol_version": 1,
		"name":             "cursor",
		"type":             "Cursor",
		"description":      "Cursor - AI-powered code editor",
		"is_preview":       true,
		"protected_dirs":   []string{".cursor"},
		"hook_names": []string{
			hookNameSessionStart,
			hookNameSessionEnd,
			hookNameBeforeSubmitPrompt,
			hookNameStop,
			hookNamePreCompact,
			hookNameSubagentStart,
			hookNameSubagentStop,
		},
		"capabilities": map[string]bool{
			"hooks":                    true,
			"transcript_analyzer":      true,
			"transcript_preparer":      false,
			"token_calculator":         false,
			"text_generator":           false,
			"hook_response_writer":     false,
			"subagent_aware_extractor": false,
		},
	})
}

func cmdDetect() error {
	root, err := repoRoot()
	if err != nil {
		root = "."
	}

	cursorDir := filepath.Join(root, ".cursor")
	if _, err := os.Stat(cursorDir); err == nil {
		return writeJSON(map[string]any{"present": true})
	}
	return writeJSON(map[string]any{"present": false})
}
