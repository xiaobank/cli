package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const agentName = "cursor"

// agentSessionJSON mirrors the external protocol's AgentSessionJSON type.
type agentSessionJSON struct {
	SessionID     string   `json:"session_id"`
	AgentName     string   `json:"agent_name"`
	RepoPath      string   `json:"repo_path"`
	SessionRef    string   `json:"session_ref"`
	StartTime     string   `json:"start_time"`
	NativeData    []byte   `json:"native_data"`
	ModifiedFiles []string `json:"modified_files"`
	NewFiles      []string `json:"new_files"`
	DeletedFiles  []string `json:"deleted_files"`
}

// hookInputJSON mirrors the external protocol's HookInputJSON type.
type hookInputJSON struct {
	HookType   string                 `json:"hook_type"`
	SessionID  string                 `json:"session_id"`
	SessionRef string                 `json:"session_ref"`
	Timestamp  string                 `json:"timestamp"`
	UserPrompt string                 `json:"user_prompt,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolUseID  string                 `json:"tool_use_id,omitempty"`
	ToolInput  json.RawMessage        `json:"tool_input,omitempty"`
	RawData    map[string]interface{} `json:"raw_data,omitempty"`
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]`)

func sanitizePathForCursor(path string) string {
	path = strings.TrimLeft(path, "/")
	return nonAlphanumericRegex.ReplaceAllString(path, "-")
}

// cmdDetect checks if .cursor/ directory exists in the repo root.
func cmdDetect() error {
	repoRoot := os.Getenv("ENTIRE_REPO_ROOT")
	if repoRoot == "" {
		repoRoot = "."
	}

	cursorDir := filepath.Join(repoRoot, ".cursor")
	_, err := os.Stat(cursorDir) //nolint:gosec // path constructed from env + fixed suffix
	present := err == nil

	return writeJSON(map[string]bool{"present": present})
}

// cmdGetSessionID extracts session_id from the HookInputJSON on stdin.
func cmdGetSessionID() error {
	var input hookInputJSON
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return fmt.Errorf("failed to parse hook input: %w", err)
	}
	return writeJSON(map[string]string{"session_id": input.SessionID})
}

// cmdGetSessionDir returns the cursor transcript directory for the given repo.
func cmdGetSessionDir(repoPath string) error {
	if override := os.Getenv("ENTIRE_TEST_CURSOR_PROJECT_DIR"); override != "" {
		return writeJSON(map[string]string{"session_dir": override})
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := sanitizePathForCursor(repoPath)
	sessionDir := filepath.Join(homeDir, ".cursor", "projects", projectDir, "agent-transcripts")
	return writeJSON(map[string]string{"session_dir": sessionDir})
}

// cmdResolveSessionFile returns the path to the session file.
// Cursor IDE uses nested: <dir>/<id>/<id>.jsonl
// Cursor CLI uses flat: <dir>/<id>.jsonl
func cmdResolveSessionFile(sessionDir, sessionID string) error {
	nestedDir := filepath.Join(sessionDir, sessionID)
	nested := filepath.Join(nestedDir, sessionID+".jsonl")
	if _, err := os.Stat(nested); err == nil {
		return writeJSON(map[string]string{"session_file": nested})
	}
	// IDE creates the directory before the transcript file
	if info, err := os.Stat(nestedDir); err == nil && info.IsDir() {
		return writeJSON(map[string]string{"session_file": nested})
	}
	flat := filepath.Join(sessionDir, sessionID+".jsonl")
	return writeJSON(map[string]string{"session_file": flat})
}

// cmdReadSession reads a session from Cursor's JSONL transcript file.
func cmdReadSession() error {
	var input hookInputJSON
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return fmt.Errorf("failed to parse hook input: %w", err)
	}

	if input.SessionRef == "" {
		return errors.New("session reference (transcript path) is required")
	}

	data, err := os.ReadFile(input.SessionRef)
	if err != nil {
		return fmt.Errorf("failed to read transcript: %w", err)
	}

	session := agentSessionJSON{
		SessionID:  input.SessionID,
		AgentName:  agentName,
		SessionRef: input.SessionRef,
		StartTime:  time.Now().Format(time.RFC3339),
		NativeData: data,
	}
	return writeJSON(session)
}

// cmdWriteSession writes native data to the session ref path.
func cmdWriteSession() error {
	var session agentSessionJSON
	if err := json.NewDecoder(os.Stdin).Decode(&session); err != nil {
		return fmt.Errorf("failed to parse session: %w", err)
	}

	if session.SessionRef == "" {
		return errors.New("session reference (transcript path) is required")
	}

	if len(session.NativeData) == 0 {
		return errors.New("session has no native data to write")
	}

	if session.AgentName != "" && session.AgentName != agentName {
		return fmt.Errorf("session belongs to agent %q, not %q", session.AgentName, agentName)
	}

	if err := os.WriteFile(session.SessionRef, session.NativeData, 0o600); err != nil {
		return fmt.Errorf("failed to write transcript: %w", err)
	}

	return nil
}

// cmdFormatResumeCommand returns an instruction to resume a Cursor session.
func cmdFormatResumeCommand() error {
	return writeJSON(map[string]string{
		"command": "Open this project in Cursor to continue the session.",
	})
}

// cmdReadTranscript reads raw transcript bytes from the session ref.
func cmdReadTranscript(sessionRef string) error {
	data, err := os.ReadFile(sessionRef) //nolint:gosec // path from protocol input
	if err != nil {
		return fmt.Errorf("failed to read transcript: %w", err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}
