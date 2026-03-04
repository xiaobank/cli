package main

import (
	"encoding/json"
	"regexp"
)

// Event type constants matching agent.EventType values.
const (
	eventSessionStart  = 1
	eventTurnStart     = 2
	eventTurnEnd       = 3
	eventCompaction    = 4
	eventSessionEnd    = 5
	eventSubagentStart = 6
	eventSubagentEnd   = 7
)

// Cursor hook names.
const (
	hookNameSessionStart       = "session-start"
	hookNameSessionEnd         = "session-end"
	hookNameBeforeSubmitPrompt = "before-submit-prompt"
	hookNameStop               = "stop"
	hookNamePreCompact         = "pre-compact"
	hookNameSubagentStart      = "subagent-start"
	hookNameSubagentStop       = "subagent-stop"
)

// hooksFileName is the hooks file used by Cursor.
const hooksFileName = "hooks.json"

// entireHookPrefixes are command prefixes that identify Entire hooks.
var entireHookPrefixes = []string{
	"entire ",
	"go run ${CURSOR_PROJECT_DIR}/cmd/entire/main.go ",
}

// cursorHooksFile represents the .cursor/hooks.json structure.
type cursorHooksFile struct {
	Version int         `json:"version"`
	Hooks   cursorHooks `json:"hooks"`
}

// cursorHooks contains all hook configurations.
type cursorHooks struct {
	SessionStart       []cursorHookEntry `json:"sessionStart,omitempty"`
	SessionEnd         []cursorHookEntry `json:"sessionEnd,omitempty"`
	BeforeSubmitPrompt []cursorHookEntry `json:"beforeSubmitPrompt,omitempty"`
	Stop               []cursorHookEntry `json:"stop,omitempty"`
	PreCompact         []cursorHookEntry `json:"preCompact,omitempty"`
	SubagentStart      []cursorHookEntry `json:"subagentStart,omitempty"`
	SubagentStop       []cursorHookEntry `json:"subagentStop,omitempty"`
}

// cursorHookEntry represents a single hook command.
type cursorHookEntry struct {
	Command string `json:"command"`
	Matcher string `json:"matcher,omitempty"`
}

// --- Raw hook input structs ---

type sessionStartRaw struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	Model          string   `json:"model"`
	HookEventName  string   `json:"hook_event_name"`
	CursorVersion  string   `json:"cursor_version"`
	WorkspaceRoots []string `json:"workspace_roots"`
	UserEmail      string   `json:"user_email"`
	TranscriptPath string   `json:"transcript_path"`

	IsBackgroundAgent bool   `json:"is_background_agent"`
	ComposerMode      string `json:"composer_mode"`
}

type stopHookInputRaw struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	Model          string   `json:"model"`
	HookEventName  string   `json:"hook_event_name"`
	CursorVersion  string   `json:"cursor_version"`
	WorkspaceRoots []string `json:"workspace_roots"`
	UserEmail      string   `json:"user_email"`
	TranscriptPath string   `json:"transcript_path"`

	Status    string      `json:"status"`
	LoopCount json.Number `json:"loop_count"`
}

type sessionEndRaw struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	Model          string   `json:"model"`
	HookEventName  string   `json:"hook_event_name"`
	CursorVersion  string   `json:"cursor_version"`
	WorkspaceRoots []string `json:"workspace_roots"`
	UserEmail      string   `json:"user_email"`
	TranscriptPath string   `json:"transcript_path"`

	Reason            string      `json:"reason"`
	DurationMs        json.Number `json:"duration_ms"`
	IsBackgroundAgent bool        `json:"is_background_agent"`
	FinalStatus       string      `json:"final_status"`
}

type beforeSubmitPromptInputRaw struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	Model          string   `json:"model"`
	HookEventName  string   `json:"hook_event_name"`
	CursorVersion  string   `json:"cursor_version"`
	WorkspaceRoots []string `json:"workspace_roots"`
	UserEmail      string   `json:"user_email"`
	TranscriptPath string   `json:"transcript_path"`

	Prompt string `json:"prompt"`
}

type preCompactHookInputRaw struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	Model          string   `json:"model"`
	HookEventName  string   `json:"hook_event_name"`
	CursorVersion  string   `json:"cursor_version"`
	WorkspaceRoots []string `json:"workspace_roots"`
	UserEmail      string   `json:"user_email"`
	TranscriptPath string   `json:"transcript_path"`

	Trigger             string      `json:"trigger"`
	ContextUsagePercent json.Number `json:"context_usage_percent"`
	ContextTokens       json.Number `json:"context_tokens"`
	ContextWindowSize   json.Number `json:"context_window_size"`
	MessageCount        json.Number `json:"message_count"`
	MessagesToCompact   json.Number `json:"messages_to_compact"`
	IsFirstCompaction   bool        `json:"is_first_compaction"`
}

type subagentStartHookInputRaw struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	Model          string   `json:"model"`
	HookEventName  string   `json:"hook_event_name"`
	CursorVersion  string   `json:"cursor_version"`
	WorkspaceRoots []string `json:"workspace_roots"`
	UserEmail      string   `json:"user_email"`
	TranscriptPath string   `json:"transcript_path"`

	SubagentID           string `json:"subagent_id"`
	SubagentType         string `json:"subagent_type"`
	SubagentModel        string `json:"subagent_model"`
	Task                 string `json:"task"`
	ParentConversationID string `json:"parent_conversation_id"`
	ToolCallID           string `json:"tool_call_id"`
	IsParallelWorker     bool   `json:"is_parallel_worker"`
}

type subagentStopHookInputRaw struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	Model          string   `json:"model"`
	HookEventName  string   `json:"hook_event_name"`
	CursorVersion  string   `json:"cursor_version"`
	WorkspaceRoots []string `json:"workspace_roots"`
	UserEmail      string   `json:"user_email"`
	TranscriptPath string   `json:"transcript_path"`

	SubagentID           string      `json:"subagent_id"`
	SubagentType         string      `json:"subagent_type"`
	Status               string      `json:"status"`
	Duration             json.Number `json:"duration_ms"`
	Summary              string      `json:"summary"`
	ParentConversationID string      `json:"parent_conversation_id"`
	MessageCount         json.Number `json:"message_count"`
	ToolCallCount        json.Number `json:"tool_call_count"`
	ModifiedFiles        []string    `json:"modified_files"`
	LoopCount            json.Number `json:"loop_count"`
	Task                 string      `json:"task"`
	Description          string      `json:"description"`
	AgentTranscriptPath  string      `json:"agent_transcript_path"`
}

// --- IDE tag regexes (copied from textutil/ide_tags.go) ---

var ideContextTagRegex = regexp.MustCompile(`(?s)<ide_[^>]*>.*?</ide_[^>]*>`)

var systemTagRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?s)<local-command-caveat[^>]*>.*?</local-command-caveat>`),
	regexp.MustCompile(`(?s)<system-reminder[^>]*>.*?</system-reminder>`),
	regexp.MustCompile(`(?s)<command-name[^>]*>.*?</command-name>`),
	regexp.MustCompile(`(?s)<command-message[^>]*>.*?</command-message>`),
	regexp.MustCompile(`(?s)<command-args[^>]*>.*?</command-args>`),
	regexp.MustCompile(`(?s)<local-command-stdout[^>]*>.*?</local-command-stdout>`),
	regexp.MustCompile(`</?user_query>`),
}
