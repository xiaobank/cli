package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]`)

func sanitizePathForCursor(path string) string {
	path = strings.TrimLeft(path, "/")
	return nonAlphanumericRegex.ReplaceAllString(path, "-")
}

func cmdGetSessionID() error {
	var hookInput struct {
		SessionID string `json:"session_id"`
	}
	data, err := readStdin()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &hookInput); err != nil {
		return fmt.Errorf("failed to unmarshal hook input: %w", err)
	}
	return writeJSON(map[string]any{
		"session_id": hookInput.SessionID,
	})
}

func cmdGetSessionDir() error {
	fs := flag.NewFlagSet("get-session-dir", flag.ContinueOnError)
	repoPath := fs.String("repo-path", "", "repository path")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	if override := os.Getenv("ENTIRE_TEST_CURSOR_PROJECT_DIR"); override != "" {
		return writeJSON(map[string]any{"session_dir": override})
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := sanitizePathForCursor(*repoPath)
	sessionDir := filepath.Join(homeDir, ".cursor", "projects", projectDir, "agent-transcripts")
	return writeJSON(map[string]any{"session_dir": sessionDir})
}

func cmdResolveSessionFile() error {
	fs := flag.NewFlagSet("resolve-session-file", flag.ContinueOnError)
	sessionDir := fs.String("session-dir", "", "session directory")
	sessionID := fs.String("session-id", "", "session ID")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	result := resolveSessionFile(*sessionDir, *sessionID)
	return writeJSON(map[string]any{"session_file": result})
}

// resolveSessionFile returns the path to a Cursor session file.
// Cursor IDE uses nested layout: <dir>/<id>/<id>.jsonl
// Cursor CLI uses flat layout: <dir>/<id>.jsonl
// Prefers nested if file OR directory exists.
func resolveSessionFile(sessionDir, agentSessionID string) string {
	nestedDir := filepath.Join(sessionDir, agentSessionID)
	nested := filepath.Join(nestedDir, agentSessionID+".jsonl")
	if _, err := os.Stat(nested); err == nil {
		return nested
	}
	if info, err := os.Stat(nestedDir); err == nil && info.IsDir() {
		return nested
	}
	return filepath.Join(sessionDir, agentSessionID+".jsonl")
}

func cmdReadSession() error {
	data, err := readStdin()
	if err != nil {
		return err
	}

	var hookInput struct {
		SessionID  string `json:"session_id"`
		SessionRef string `json:"session_ref"`
	}
	if err := json.Unmarshal(data, &hookInput); err != nil {
		return fmt.Errorf("failed to parse hook input: %w", err)
	}

	if hookInput.SessionRef == "" {
		return errors.New("session reference (transcript path) is required")
	}

	transcriptData, err := os.ReadFile(hookInput.SessionRef)
	if err != nil {
		return fmt.Errorf("failed to read transcript: %w", err)
	}

	return writeJSON(map[string]any{
		"session_id":     hookInput.SessionID,
		"agent_name":     "cursor",
		"repo_path":      "",
		"session_ref":    hookInput.SessionRef,
		"start_time":     time.Now().Format(time.RFC3339),
		"native_data":    transcriptData,
		"modified_files": []string{},
		"new_files":      []string{},
		"deleted_files":  []string{},
	})
}

func cmdWriteSession() error {
	data, err := readStdin()
	if err != nil {
		return err
	}

	var session struct {
		AgentName  string `json:"agent_name"`
		SessionRef string `json:"session_ref"`
		NativeData []byte `json:"native_data"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("failed to parse session: %w", err)
	}

	if session.AgentName != "" && session.AgentName != "cursor" {
		return fmt.Errorf("session belongs to agent %q, not %q", session.AgentName, "cursor")
	}

	if session.SessionRef == "" {
		return errors.New("session reference (transcript path) is required")
	}

	if len(session.NativeData) == 0 {
		return errors.New("session has no native data to write")
	}

	if err := os.WriteFile(session.SessionRef, session.NativeData, 0o600); err != nil {
		return fmt.Errorf("failed to write transcript: %w", err)
	}

	return nil
}

func cmdFormatResumeCommand() error {
	fs := flag.NewFlagSet("format-resume-command", flag.ContinueOnError)
	_ = fs.String("session-id", "", "session ID")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return writeJSON(map[string]any{
		"command": "Open this project in Cursor to continue the session.",
	})
}

func cmdReadTranscript() error {
	fs := flag.NewFlagSet("read-transcript", flag.ContinueOnError)
	sessionRef := fs.String("session-ref", "", "session reference path")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	data, err := os.ReadFile(*sessionRef)
	if err != nil {
		return fmt.Errorf("failed to read transcript: %w", err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}

func cmdChunkTranscript() error {
	fs := flag.NewFlagSet("chunk-transcript", flag.ContinueOnError)
	maxSize := fs.Int("max-size", 0, "maximum chunk size in bytes")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}

	chunkSize := *maxSize
	if chunkSize <= 0 {
		chunkSize = len(content)
		if chunkSize == 0 {
			return writeJSON(map[string]any{"chunks": [][]byte{}})
		}
	}

	chunks, err := chunkJSONL(content, chunkSize)
	if err != nil {
		return fmt.Errorf("failed to chunk transcript: %w", err)
	}

	return writeJSON(map[string]any{"chunks": chunks})
}

func cmdReassembleTranscript() error {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}

	var resp struct {
		Chunks [][]byte `json:"chunks"`
	}
	if err := json.Unmarshal(input, &resp); err != nil {
		return fmt.Errorf("failed to unmarshal chunks: %w", err)
	}

	result := reassembleJSONL(resp.Chunks)
	if _, err := os.Stdout.Write(result); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}
