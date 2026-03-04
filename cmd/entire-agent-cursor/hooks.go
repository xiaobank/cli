package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdParseHook() error {
	fs := flag.NewFlagSet("parse-hook", flag.ContinueOnError)
	hookName := fs.String("hook", "", "hook name")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	switch *hookName {
	case hookNameSessionStart:
		return parseSessionStart()
	case hookNameBeforeSubmitPrompt:
		return parseTurnStart()
	case hookNameStop:
		return parseTurnEnd()
	case hookNameSessionEnd:
		return parseSessionEnd()
	case hookNamePreCompact:
		return parsePreCompact()
	case hookNameSubagentStart:
		return parseSubagentStart()
	case hookNameSubagentStop:
		return parseSubagentStop()
	default:
		// Unknown hooks have no lifecycle significance
		return writeNull()
	}
}

func parseSessionStart() error {
	raw, err := readJSON[sessionStartRaw]()
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"type":        eventSessionStart,
		"session_id":  raw.ConversationID,
		"session_ref": raw.TranscriptPath,
		"timestamp":   nowRFC3339(),
	})
}

func parseTurnStart() error {
	raw, err := readJSON[beforeSubmitPromptInputRaw]()
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"type":        eventTurnStart,
		"session_id":  raw.ConversationID,
		"session_ref": resolveTranscriptRef(raw.ConversationID, raw.TranscriptPath),
		"prompt":      raw.Prompt,
		"model":       raw.Model,
		"timestamp":   nowRFC3339(),
	})
}

func parseTurnEnd() error {
	raw, err := readJSON[stopHookInputRaw]()
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"type":        eventTurnEnd,
		"session_id":  raw.ConversationID,
		"session_ref": resolveTranscriptRef(raw.ConversationID, raw.TranscriptPath),
		"model":       raw.Model,
		"timestamp":   nowRFC3339(),
	})
}

func parseSessionEnd() error {
	raw, err := readJSON[sessionEndRaw]()
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"type":        eventSessionEnd,
		"session_id":  raw.ConversationID,
		"session_ref": resolveTranscriptRef(raw.ConversationID, raw.TranscriptPath),
		"timestamp":   nowRFC3339(),
	})
}

func parsePreCompact() error {
	raw, err := readJSON[preCompactHookInputRaw]()
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"type":        eventCompaction,
		"session_id":  raw.ConversationID,
		"session_ref": raw.TranscriptPath,
		"timestamp":   nowRFC3339(),
	})
}

func parseSubagentStart() error {
	raw, err := readJSON[subagentStartHookInputRaw]()
	if err != nil {
		return err
	}
	if raw.Task == "" {
		return writeNull()
	}
	return writeJSON(map[string]any{
		"type":             eventSubagentStart,
		"session_id":       raw.ConversationID,
		"session_ref":      raw.TranscriptPath,
		"subagent_id":      raw.SubagentID,
		"tool_use_id":      raw.SubagentID,
		"subagent_type":    raw.SubagentType,
		"task_description": raw.Task,
		"timestamp":        nowRFC3339(),
	})
}

func parseSubagentStop() error {
	raw, err := readJSON[subagentStopHookInputRaw]()
	if err != nil {
		return err
	}
	if raw.Task == "" {
		return writeNull()
	}
	return writeJSON(map[string]any{
		"type":             eventSubagentEnd,
		"session_id":       raw.ConversationID,
		"session_ref":      raw.TranscriptPath,
		"tool_use_id":      raw.SubagentID,
		"subagent_id":      raw.SubagentID,
		"subagent_type":    raw.SubagentType,
		"task_description": raw.Task,
		"timestamp":        nowRFC3339(),
	})
}

// resolveTranscriptRef returns the transcript path, computing it dynamically
// when the hook doesn't provide one (Cursor CLI pattern).
func resolveTranscriptRef(conversationID, rawPath string) string {
	if rawPath != "" {
		return rawPath
	}

	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cursor: failed to get repo root for transcript resolution: %v\n", err)
		return ""
	}

	sessionDir, err := getSessionDir(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cursor: failed to get session dir for transcript resolution: %v\n", err)
		return ""
	}

	return resolveSessionFile(sessionDir, conversationID)
}

// getSessionDir returns the Cursor session directory for a given repo path.
func getSessionDir(repoPath string) (string, error) {
	if override := os.Getenv("ENTIRE_TEST_CURSOR_PROJECT_DIR"); override != "" {
		return override, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := sanitizePathForCursor(repoPath)
	return filepath.Join(homeDir, ".cursor", "projects", projectDir, "agent-transcripts"), nil
}

// --- Install/Uninstall hooks ---

func cmdInstallHooks() error {
	fs := flag.NewFlagSet("install-hooks", flag.ContinueOnError)
	localDev := fs.Bool("local-dev", false, "use local dev command prefix")
	force := fs.Bool("force", false, "force reinstall")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	root, err := repoRoot()
	if err != nil {
		root = "."
	}

	hooksPath := filepath.Join(root, ".cursor", hooksFileName)

	// Use raw maps to preserve unknown fields on round-trip
	var rawFile map[string]json.RawMessage
	var rawHooks map[string]json.RawMessage

	existingData, readErr := os.ReadFile(hooksPath) //nolint:gosec // path constructed from repo root
	if readErr == nil {
		if err := json.Unmarshal(existingData, &rawFile); err != nil {
			return fmt.Errorf("failed to parse existing %s: %w", hooksFileName, err)
		}
		if hooksRaw, ok := rawFile["hooks"]; ok {
			if err := json.Unmarshal(hooksRaw, &rawHooks); err != nil {
				return fmt.Errorf("failed to parse hooks in %s: %w", hooksFileName, err)
			}
		}
		if _, ok := rawFile["version"]; !ok {
			rawFile["version"] = json.RawMessage(`1`)
		}
	} else {
		rawFile = map[string]json.RawMessage{
			"version": json.RawMessage(`1`),
		}
	}

	if rawHooks == nil {
		rawHooks = make(map[string]json.RawMessage)
	}

	// Parse only the hook types we manage
	var sessionStart, sessionEnd, beforeSubmitPrompt, stop, preCompact, subagentStart, subagentStop []cursorHookEntry
	parseCursorHookType(rawHooks, "sessionStart", &sessionStart)
	parseCursorHookType(rawHooks, "sessionEnd", &sessionEnd)
	parseCursorHookType(rawHooks, "beforeSubmitPrompt", &beforeSubmitPrompt)
	parseCursorHookType(rawHooks, "stop", &stop)
	parseCursorHookType(rawHooks, "preCompact", &preCompact)
	parseCursorHookType(rawHooks, "subagentStart", &subagentStart)
	parseCursorHookType(rawHooks, "subagentStop", &subagentStop)

	if *force {
		sessionStart = removeEntireHooks(sessionStart)
		sessionEnd = removeEntireHooks(sessionEnd)
		beforeSubmitPrompt = removeEntireHooks(beforeSubmitPrompt)
		stop = removeEntireHooks(stop)
		preCompact = removeEntireHooks(preCompact)
		subagentStart = removeEntireHooks(subagentStart)
		subagentStop = removeEntireHooks(subagentStop)
	}

	var cmdPrefix string
	if *localDev {
		cmdPrefix = "go run ${CURSOR_PROJECT_DIR}/cmd/entire/main.go hooks cursor "
	} else {
		cmdPrefix = "entire hooks cursor "
	}

	sessionStartCmd := cmdPrefix + hookNameSessionStart
	sessionEndCmd := cmdPrefix + hookNameSessionEnd
	beforeSubmitPromptCmd := cmdPrefix + hookNameBeforeSubmitPrompt
	stopCmd := cmdPrefix + hookNameStop
	preCompactCmd := cmdPrefix + hookNamePreCompact
	subagentStartCmd := cmdPrefix + hookNameSubagentStart
	subagentEndCmd := cmdPrefix + hookNameSubagentStop

	count := 0

	if !hookCommandExists(sessionStart, sessionStartCmd) {
		sessionStart = append(sessionStart, cursorHookEntry{Command: sessionStartCmd})
		count++
	}
	if !hookCommandExists(sessionEnd, sessionEndCmd) {
		sessionEnd = append(sessionEnd, cursorHookEntry{Command: sessionEndCmd})
		count++
	}
	if !hookCommandExists(beforeSubmitPrompt, beforeSubmitPromptCmd) {
		beforeSubmitPrompt = append(beforeSubmitPrompt, cursorHookEntry{Command: beforeSubmitPromptCmd})
		count++
	}
	if !hookCommandExists(stop, stopCmd) {
		stop = append(stop, cursorHookEntry{Command: stopCmd})
		count++
	}
	if !hookCommandExists(preCompact, preCompactCmd) {
		preCompact = append(preCompact, cursorHookEntry{Command: preCompactCmd})
		count++
	}
	if !hookCommandExists(subagentStart, subagentStartCmd) {
		subagentStart = append(subagentStart, cursorHookEntry{Command: subagentStartCmd})
		count++
	}
	if !hookCommandExists(subagentStop, subagentEndCmd) {
		subagentStop = append(subagentStop, cursorHookEntry{Command: subagentEndCmd})
		count++
	}

	if count == 0 {
		return writeJSON(map[string]any{"hooks_installed": 0})
	}

	marshalCursorHookType(rawHooks, "sessionStart", sessionStart)
	marshalCursorHookType(rawHooks, "sessionEnd", sessionEnd)
	marshalCursorHookType(rawHooks, "beforeSubmitPrompt", beforeSubmitPrompt)
	marshalCursorHookType(rawHooks, "stop", stop)
	marshalCursorHookType(rawHooks, "preCompact", preCompact)
	marshalCursorHookType(rawHooks, "subagentStart", subagentStart)
	marshalCursorHookType(rawHooks, "subagentStop", subagentStop)

	hooksJSON, err := json.Marshal(rawHooks)
	if err != nil {
		return fmt.Errorf("failed to marshal hooks: %w", err)
	}
	rawFile["hooks"] = hooksJSON

	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o750); err != nil {
		return fmt.Errorf("failed to create .cursor directory: %w", err)
	}

	output, err := marshalIndentWithNewline(rawFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", hooksFileName, err)
	}

	if err := os.WriteFile(hooksPath, output, 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", hooksFileName, err)
	}

	return writeJSON(map[string]any{"hooks_installed": count})
}

func cmdUninstallHooks() error {
	root, err := repoRoot()
	if err != nil {
		root = "."
	}

	hooksPath := filepath.Join(root, ".cursor", hooksFileName)
	data, err := os.ReadFile(hooksPath) //nolint:gosec // path constructed from repo root
	if err != nil {
		return nil //nolint:nilerr // No hooks file means nothing to uninstall
	}

	var rawFile map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawFile); err != nil {
		return fmt.Errorf("failed to parse %s: %w", hooksFileName, err)
	}

	var rawHooks map[string]json.RawMessage
	if hooksRaw, ok := rawFile["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &rawHooks); err != nil {
			return fmt.Errorf("failed to parse hooks in %s: %w", hooksFileName, err)
		}
	}
	if rawHooks == nil {
		rawHooks = make(map[string]json.RawMessage)
	}

	var sessionStart, sessionEnd, beforeSubmitPrompt, stop, preCompact, subagentStart, subagentStop []cursorHookEntry
	parseCursorHookType(rawHooks, "sessionStart", &sessionStart)
	parseCursorHookType(rawHooks, "sessionEnd", &sessionEnd)
	parseCursorHookType(rawHooks, "beforeSubmitPrompt", &beforeSubmitPrompt)
	parseCursorHookType(rawHooks, "stop", &stop)
	parseCursorHookType(rawHooks, "preCompact", &preCompact)
	parseCursorHookType(rawHooks, "subagentStart", &subagentStart)
	parseCursorHookType(rawHooks, "subagentStop", &subagentStop)

	sessionStart = removeEntireHooks(sessionStart)
	sessionEnd = removeEntireHooks(sessionEnd)
	beforeSubmitPrompt = removeEntireHooks(beforeSubmitPrompt)
	stop = removeEntireHooks(stop)
	preCompact = removeEntireHooks(preCompact)
	subagentStart = removeEntireHooks(subagentStart)
	subagentStop = removeEntireHooks(subagentStop)

	marshalCursorHookType(rawHooks, "sessionStart", sessionStart)
	marshalCursorHookType(rawHooks, "sessionEnd", sessionEnd)
	marshalCursorHookType(rawHooks, "beforeSubmitPrompt", beforeSubmitPrompt)
	marshalCursorHookType(rawHooks, "stop", stop)
	marshalCursorHookType(rawHooks, "preCompact", preCompact)
	marshalCursorHookType(rawHooks, "subagentStart", subagentStart)
	marshalCursorHookType(rawHooks, "subagentStop", subagentStop)

	if len(rawHooks) > 0 {
		hooksJSON, err := json.Marshal(rawHooks)
		if err != nil {
			return fmt.Errorf("failed to marshal hooks: %w", err)
		}
		rawFile["hooks"] = hooksJSON
	} else {
		delete(rawFile, "hooks")
	}

	output, err := marshalIndentWithNewline(rawFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", hooksFileName, err)
	}

	if err := os.WriteFile(hooksPath, output, 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", hooksFileName, err)
	}
	return nil
}

func cmdAreHooksInstalled() error {
	root, err := repoRoot()
	if err != nil {
		root = "."
	}

	hooksPath := filepath.Join(root, ".cursor", hooksFileName)
	data, err := os.ReadFile(hooksPath) //nolint:gosec // path constructed from repo root
	if err != nil {
		return writeJSON(map[string]any{"installed": false})
	}

	var hf cursorHooksFile
	if err := json.Unmarshal(data, &hf); err != nil {
		return writeJSON(map[string]any{"installed": false})
	}

	installed := hasEntireHook(hf.Hooks.SessionStart) ||
		hasEntireHook(hf.Hooks.SessionEnd) ||
		hasEntireHook(hf.Hooks.BeforeSubmitPrompt) ||
		hasEntireHook(hf.Hooks.Stop) ||
		hasEntireHook(hf.Hooks.PreCompact) ||
		hasEntireHook(hf.Hooks.SubagentStart) ||
		hasEntireHook(hf.Hooks.SubagentStop)

	return writeJSON(map[string]any{"installed": installed})
}

// --- Hook helper functions ---

func parseCursorHookType(rawHooks map[string]json.RawMessage, hookType string, target *[]cursorHookEntry) {
	if data, ok := rawHooks[hookType]; ok {
		_ = json.Unmarshal(data, target) //nolint:errcheck // intentionally ignore parse errors
	}
}

func marshalCursorHookType(rawHooks map[string]json.RawMessage, hookType string, entries []cursorHookEntry) {
	if len(entries) == 0 {
		delete(rawHooks, hookType)
		return
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return
	}
	rawHooks[hookType] = data
}

func hookCommandExists(entries []cursorHookEntry, command string) bool {
	for _, entry := range entries {
		if entry.Command == command {
			return true
		}
	}
	return false
}

func isEntireHook(command string) bool {
	for _, prefix := range entireHookPrefixes {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func hasEntireHook(entries []cursorHookEntry) bool {
	for _, entry := range entries {
		if isEntireHook(entry.Command) {
			return true
		}
	}
	return false
}

func removeEntireHooks(entries []cursorHookEntry) []cursorHookEntry {
	result := make([]cursorHookEntry, 0, len(entries))
	for _, entry := range entries {
		if !isEntireHook(entry.Command) {
			result = append(result, entry)
		}
	}
	return result
}
