// Package cursor implements the Agent interface for Cursor.
package cursor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/entireio/cli/cmd/entire/cli/transcript"
)

//nolint:gochecknoinits // Agent self-registration is the intended pattern
func init() {
	agent.Register(agent.AgentNameCursor, NewCursorAgent)
}

// CursorAgent implements the Agent interface for Cursor.
//
//nolint:revive // CursorAgent is clearer than Agent in this context
type CursorAgent struct{}

// NewCursorAgent creates a new Cursor agent instance.
func NewCursorAgent() agent.Agent {
	return &CursorAgent{}
}

// Name returns the agent registry key.
func (c *CursorAgent) Name() agent.AgentName {
	return agent.AgentNameCursor
}

// Type returns the agent type identifier.
func (c *CursorAgent) Type() agent.AgentType {
	return agent.AgentTypeCursor
}

// Description returns a human-readable description.
func (c *CursorAgent) Description() string {
	return "Cursor - AI-powered code editor"
}

// DetectPresence checks if Cursor is configured in the repository.
func (c *CursorAgent) DetectPresence() (bool, error) {
	repoRoot, err := paths.RepoRoot()
	if err != nil {
		repoRoot = "."
	}

	cursorDir := filepath.Join(repoRoot, ".cursor")
	if _, err := os.Stat(cursorDir); err == nil {
		return true, nil
	}
	return false, nil
}

// GetHookConfigPath returns the path to Cursor's hook config file.
func (c *CursorAgent) GetHookConfigPath() string {
	return ".cursor/" + HooksFileName
}

// SupportsHooks returns true as Cursor supports lifecycle hooks.
func (c *CursorAgent) SupportsHooks() bool {
	return true
}

// ParseHookInput parses Cursor hook input from stdin.
func (c *CursorAgent) ParseHookInput(hookType agent.HookType, reader io.Reader) (*agent.HookInput, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	if len(data) == 0 {
		return nil, errors.New("empty input")
	}

	input := &agent.HookInput{
		HookType:  hookType,
		Timestamp: time.Now(),
		RawData:   make(map[string]interface{}),
	}

	switch hookType {
	case agent.HookUserPromptSubmit:
		var raw userPromptSubmitRaw
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse user prompt submit: %w", err)
		}
		input.SessionID = raw.getSessionID()
		input.SessionRef = raw.TranscriptPath
		input.UserPrompt = raw.Prompt

	case agent.HookSessionStart, agent.HookSessionEnd, agent.HookStop:
		var raw sessionInfoRaw
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse session info: %w", err)
		}
		input.SessionID = raw.getSessionID()
		input.SessionRef = raw.TranscriptPath

	case agent.HookPreToolUse:
		var raw taskHookInputRaw
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse pre-tool input: %w", err)
		}
		input.SessionID = raw.getSessionID()
		input.SessionRef = raw.TranscriptPath
		input.ToolUseID = raw.ToolUseID
		input.ToolInput = raw.ToolInput

	case agent.HookPostToolUse:
		var raw postToolHookInputRaw
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse post-tool input: %w", err)
		}
		input.SessionID = raw.getSessionID()
		input.SessionRef = raw.TranscriptPath
		input.ToolUseID = raw.ToolUseID
		input.ToolInput = raw.ToolInput
		if raw.ToolResponse.AgentID != "" {
			input.RawData["agent_id"] = raw.ToolResponse.AgentID
		}
	}

	return input, nil
}

// GetSessionID extracts the session ID from hook input.
func (c *CursorAgent) GetSessionID(input *agent.HookInput) string {
	return input.SessionID
}

// ResolveSessionFile returns the path to a Cursor session file.
// Cursor uses JSONL format like Claude Code.
func (c *CursorAgent) ResolveSessionFile(sessionDir, agentSessionID string) string {
	return filepath.Join(sessionDir, agentSessionID+".jsonl")
}

// ProtectedDirs returns directories that Cursor uses for config/state.
func (c *CursorAgent) ProtectedDirs() []string { return []string{".cursor"} }

// GetSessionDir returns the directory where Cursor stores session transcripts.
func (c *CursorAgent) GetSessionDir(repoPath string) (string, error) {
	if override := os.Getenv("ENTIRE_TEST_CURSOR_PROJECT_DIR"); override != "" {
		return override, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	projectDir := sanitizePathForCursor(repoPath)
	return filepath.Join(homeDir, ".cursor", "projects", projectDir), nil
}

// ReadSession reads a session from Cursor's storage (JSONL transcript file).
func (c *CursorAgent) ReadSession(input *agent.HookInput) (*agent.AgentSession, error) {
	if input.SessionRef == "" {
		return nil, errors.New("session reference (transcript path) is required")
	}

	data, err := os.ReadFile(input.SessionRef)
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript: %w", err)
	}

	lines, err := transcript.ParseFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transcript: %w", err)
	}

	return &agent.AgentSession{
		SessionID:     input.SessionID,
		AgentName:     c.Name(),
		SessionRef:    input.SessionRef,
		StartTime:     time.Now(),
		NativeData:    data,
		ModifiedFiles: extractModifiedFiles(lines),
	}, nil
}

// WriteSession writes a session to Cursor's storage (JSONL transcript file).
func (c *CursorAgent) WriteSession(session *agent.AgentSession) error {
	if session == nil {
		return errors.New("session is nil")
	}

	if session.AgentName != "" && session.AgentName != c.Name() {
		return fmt.Errorf("session belongs to agent %q, not %q", session.AgentName, c.Name())
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

// FormatResumeCommand returns the command to resume a Cursor session.
func (c *CursorAgent) FormatResumeCommand(sessionID string) string {
	return "cursor --resume " + sessionID
}

// sanitizePathForCursor converts a path to Cursor's project directory format.
var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]`)

func sanitizePathForCursor(path string) string {
	return nonAlphanumericRegex.ReplaceAllString(path, "-")
}

// ChunkTranscript splits a JSONL transcript at line boundaries.
func (c *CursorAgent) ChunkTranscript(content []byte, maxSize int) ([][]byte, error) {
	chunks, err := agent.ChunkJSONL(content, maxSize)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk JSONL transcript: %w", err)
	}
	return chunks, nil
}

// ReassembleTranscript concatenates JSONL chunks with newlines.
func (c *CursorAgent) ReassembleTranscript(chunks [][]byte) ([]byte, error) {
	return agent.ReassembleJSONL(chunks), nil
}

// extractModifiedFiles extracts file paths from transcript lines that contain file-modifying tools.
func extractModifiedFiles(lines []transcript.Line) []string {
	seen := make(map[string]bool)
	var files []string

	for i := range lines {
		if lines[i].Role != transcript.TypeAssistant && lines[i].Type != transcript.TypeAssistant {
			continue
		}

		var msg transcript.AssistantMessage
		if err := json.Unmarshal(lines[i].Message, &msg); err != nil {
			continue
		}

		for _, block := range msg.Content {
			if block.Type != transcript.ContentTypeToolUse {
				continue
			}

			isModifyTool := false
			for _, name := range FileModificationTools {
				if block.Name == name {
					isModifyTool = true
					break
				}
			}
			if !isModifyTool {
				continue
			}

			var toolInput transcript.ToolInput
			if err := json.Unmarshal(block.Input, &toolInput); err != nil {
				continue
			}

			file := toolInput.FilePath
			if file == "" {
				file = toolInput.NotebookPath
			}
			if file != "" && !seen[file] {
				seen[file] = true
				files = append(files, file)
			}
		}
	}

	return files
}
