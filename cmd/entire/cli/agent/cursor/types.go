package cursor

import "encoding/json"

// CursorHooksFile represents the .cursor/HooksFileName structure.
// Cursor uses a flat JSON file with version and hooks sections.
//
//nolint:revive // CursorHooksFile is clearer than HooksFile when used outside this package
type CursorHooksFile struct {
	Version int         `json:"version"`
	Hooks   CursorHooks `json:"hooks"`
}

// CursorHooks contains all hook configurations using camelCase keys.
//
//nolint:revive // CursorHooks is clearer than Hooks when used outside this package
type CursorHooks struct {
	SessionStart       []CursorHookEntry `json:"sessionStart,omitempty"`
	SessionEnd         []CursorHookEntry `json:"sessionEnd,omitempty"`
	BeforeSubmitPrompt []CursorHookEntry `json:"beforeSubmitPrompt,omitempty"`
	Stop               []CursorHookEntry `json:"stop,omitempty"`
	PreToolUse         []CursorHookEntry `json:"preToolUse,omitempty"`
	PostToolUse        []CursorHookEntry `json:"postToolUse,omitempty"`
}

// CursorHookEntry represents a single hook command.
// Cursor hooks have a command string and an optional matcher field for filtering by tool name.
//
//nolint:revive // CursorHookEntry is clearer than HookEntry when used outside this package
type CursorHookEntry struct {
	Command string `json:"command"`
	Matcher string `json:"matcher,omitempty"`
}

// sessionInfoRaw is the JSON structure from SessionStart/SessionEnd/Stop hooks.
// Cursor may provide session_id or conversation_id (fallback).
type sessionInfoRaw struct {
	SessionID      string `json:"session_id"`
	ConversationID string `json:"conversation_id"`
	TranscriptPath string `json:"transcript_path"`
}

// getSessionID returns session_id if present, falling back to conversation_id.
func (s *sessionInfoRaw) getSessionID() string {
	if s.SessionID != "" {
		return s.SessionID
	}
	return s.ConversationID
}

// userPromptSubmitRaw is the JSON structure from BeforeSubmitPrompt hooks.
type userPromptSubmitRaw struct {
	SessionID      string `json:"session_id"`
	ConversationID string `json:"conversation_id"`
	TranscriptPath string `json:"transcript_path"`
	Prompt         string `json:"prompt"`
}

// getSessionID returns session_id if present, falling back to conversation_id.
func (u *userPromptSubmitRaw) getSessionID() string {
	if u.SessionID != "" {
		return u.SessionID
	}
	return u.ConversationID
}

// taskHookInputRaw is the JSON structure from PreToolUse[Task] hook.
type taskHookInputRaw struct {
	SessionID      string          `json:"session_id"`
	ConversationID string          `json:"conversation_id"`
	TranscriptPath string          `json:"transcript_path"`
	ToolUseID      string          `json:"tool_use_id"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

// getSessionID returns session_id if present, falling back to conversation_id.
func (t *taskHookInputRaw) getSessionID() string {
	if t.SessionID != "" {
		return t.SessionID
	}
	return t.ConversationID
}

// postToolHookInputRaw is the JSON structure from PostToolUse hooks.
type postToolHookInputRaw struct {
	SessionID      string          `json:"session_id"`
	ConversationID string          `json:"conversation_id"`
	TranscriptPath string          `json:"transcript_path"`
	ToolUseID      string          `json:"tool_use_id"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   struct {
		AgentID string `json:"agentId"`
	} `json:"tool_response"`
}

// getSessionID returns session_id if present, falling back to conversation_id.
func (p *postToolHookInputRaw) getSessionID() string {
	if p.SessionID != "" {
		return p.SessionID
	}
	return p.ConversationID
}

// Tool names used in Cursor transcripts (same as Claude Code)
const (
	ToolWrite        = "Write"
	ToolEdit         = "Edit"
	ToolNotebookEdit = "NotebookEdit"
)

// FileModificationTools lists tools that create or modify files
var FileModificationTools = []string{
	ToolWrite,
	ToolEdit,
	ToolNotebookEdit,
}
