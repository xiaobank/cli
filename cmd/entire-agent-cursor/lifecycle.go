package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Event type constants matching agent.EventType iota values.
const (
	eventSessionStart  = 1
	eventTurnStart     = 2
	eventTurnEnd       = 3
	eventCompaction    = 4
	eventSessionEnd    = 5
	eventSubagentStart = 6
	eventSubagentEnd   = 7
)

// eventJSON is the protocol's event representation.
type eventJSON struct {
	Type              int               `json:"type"`
	SessionID         string            `json:"session_id"`
	PreviousSessionID string            `json:"previous_session_id,omitempty"`
	SessionRef        string            `json:"session_ref,omitempty"`
	Prompt            string            `json:"prompt,omitempty"`
	Model             string            `json:"model,omitempty"`
	Timestamp         string            `json:"timestamp,omitempty"`
	ToolUseID         string            `json:"tool_use_id,omitempty"`
	SubagentID        string            `json:"subagent_id,omitempty"`
	ToolInput         json.RawMessage   `json:"tool_input,omitempty"`
	SubagentType      string            `json:"subagent_type,omitempty"`
	TaskDescription   string            `json:"task_description,omitempty"`
	ResponseMessage   string            `json:"response_message,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// cmdParseHook dispatches to the appropriate hook parser.
func cmdParseHook(hookName string) error {
	switch hookName {
	case hookNameSessionStart:
		return parseSessionStart(os.Stdin)
	case hookNameBeforeSubmitPrompt:
		return parseTurnStart(os.Stdin)
	case hookNameStop:
		return parseTurnEnd(os.Stdin)
	case hookNameSessionEnd:
		return parseSessionEnd(os.Stdin)
	case hookNamePreCompact:
		return parsePreCompact(os.Stdin)
	case hookNameSubagentStart:
		return parseSubagentStart(os.Stdin)
	case hookNameSubagentStop:
		return parseSubagentStop(os.Stdin)
	default:
		return writeNull()
	}
}

// resolveTranscriptRef returns the transcript path, or computes it dynamically
// when the hook doesn't provide one (Cursor CLI pattern).
func resolveTranscriptRef(conversationID, rawPath string) string {
	if rawPath != "" {
		return rawPath
	}

	repoRoot := os.Getenv("ENTIRE_REPO_ROOT")
	if repoRoot == "" {
		return ""
	}

	sessionDir, err := getSessionDir(repoRoot)
	if err != nil {
		return ""
	}

	return resolveSessionFile(sessionDir, conversationID)
}

// getSessionDir computes the session directory for a repo path.
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

// resolveSessionFile returns the resolved session file path.
func resolveSessionFile(sessionDir, sessionID string) string {
	nestedDir := filepath.Join(sessionDir, sessionID)
	nested := filepath.Join(nestedDir, sessionID+".jsonl")
	if _, err := os.Stat(nested); err == nil { //nolint:gosec // path from protocol input
		return nested
	}
	if info, err := os.Stat(nestedDir); err == nil && info.IsDir() { //nolint:gosec // path from protocol input
		return nested
	}
	return filepath.Join(sessionDir, sessionID+".jsonl")
}

func readAndParse[T any](stdin io.Reader) (*T, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("failed to read hook input: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("empty hook input")
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse hook input: %w", err)
	}
	return &result, nil
}

func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}

func parseSessionStart(stdin io.Reader) error {
	raw, err := readAndParse[sessionStartRaw](stdin)
	if err != nil {
		return err
	}
	return writeJSON(eventJSON{
		Type:       eventSessionStart,
		SessionID:  raw.ConversationID,
		SessionRef: raw.TranscriptPath,
		Timestamp:  nowRFC3339(),
	})
}

func parseTurnStart(stdin io.Reader) error {
	raw, err := readAndParse[beforeSubmitPromptInputRaw](stdin)
	if err != nil {
		return err
	}
	return writeJSON(eventJSON{
		Type:       eventTurnStart,
		SessionID:  raw.ConversationID,
		SessionRef: resolveTranscriptRef(raw.ConversationID, raw.TranscriptPath),
		Prompt:     raw.Prompt,
		Model:      raw.Model,
		Timestamp:  nowRFC3339(),
	})
}

func parseTurnEnd(stdin io.Reader) error {
	raw, err := readAndParse[stopHookInputRaw](stdin)
	if err != nil {
		return err
	}
	return writeJSON(eventJSON{
		Type:       eventTurnEnd,
		SessionID:  raw.ConversationID,
		SessionRef: resolveTranscriptRef(raw.ConversationID, raw.TranscriptPath),
		Model:      raw.Model,
		Timestamp:  nowRFC3339(),
	})
}

func parseSessionEnd(stdin io.Reader) error {
	raw, err := readAndParse[sessionEndRaw](stdin)
	if err != nil {
		return err
	}
	return writeJSON(eventJSON{
		Type:       eventSessionEnd,
		SessionID:  raw.ConversationID,
		SessionRef: resolveTranscriptRef(raw.ConversationID, raw.TranscriptPath),
		Timestamp:  nowRFC3339(),
	})
}

func parsePreCompact(stdin io.Reader) error {
	raw, err := readAndParse[preCompactHookInputRaw](stdin)
	if err != nil {
		return err
	}
	return writeJSON(eventJSON{
		Type:       eventCompaction,
		SessionID:  raw.ConversationID,
		SessionRef: raw.TranscriptPath,
		Timestamp:  nowRFC3339(),
	})
}

func parseSubagentStart(stdin io.Reader) error {
	raw, err := readAndParse[subagentStartHookInputRaw](stdin)
	if err != nil {
		return err
	}
	if raw.Task == "" {
		return writeNull()
	}
	return writeJSON(eventJSON{
		Type:            eventSubagentStart,
		SessionID:       raw.ConversationID,
		SessionRef:      raw.TranscriptPath,
		SubagentID:      raw.SubagentID,
		ToolUseID:       raw.SubagentID,
		SubagentType:    raw.SubagentType,
		TaskDescription: raw.Task,
		Timestamp:       nowRFC3339(),
	})
}

func parseSubagentStop(stdin io.Reader) error {
	raw, err := readAndParse[subagentStopHookInputRaw](stdin)
	if err != nil {
		return err
	}
	if raw.Task == "" {
		return writeNull()
	}
	return writeJSON(eventJSON{
		Type:            eventSubagentEnd,
		SessionID:       raw.ConversationID,
		SessionRef:      raw.TranscriptPath,
		ToolUseID:       raw.SubagentID,
		SubagentID:      raw.SubagentID,
		SubagentType:    raw.SubagentType,
		TaskDescription: raw.Task,
		Timestamp:       nowRFC3339(),
	})
}
