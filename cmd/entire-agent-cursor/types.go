package main

import "encoding/json"

// CursorHooksFile represents the .cursor/hooks.json structure.
type CursorHooksFile struct {
	Version int         `json:"version"`
	Hooks   CursorHooks `json:"hooks"`
}

// CursorHooks contains all hook configurations using camelCase keys.
type CursorHooks struct {
	SessionStart       []CursorHookEntry `json:"sessionStart,omitempty"`
	SessionEnd         []CursorHookEntry `json:"sessionEnd,omitempty"`
	BeforeSubmitPrompt []CursorHookEntry `json:"beforeSubmitPrompt,omitempty"`
	Stop               []CursorHookEntry `json:"stop,omitempty"`
	PreCompact         []CursorHookEntry `json:"preCompact,omitempty"`
	SubagentStart      []CursorHookEntry `json:"subagentStart,omitempty"`
	SubagentStop       []CursorHookEntry `json:"subagentStop,omitempty"`
}

// CursorHookEntry represents a single hook command.
type CursorHookEntry struct {
	Command string `json:"command"`
	Matcher string `json:"matcher,omitempty"`
}

// sessionStartRaw is the JSON structure from SessionStart hooks.
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

// stopHookInputRaw is the JSON structure from Stop hooks.
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

// sessionEndRaw is the JSON structure from SessionEnd hooks.
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

// beforeSubmitPromptInputRaw is the JSON structure from BeforeSubmitPrompt hooks.
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

// preCompactHookInputRaw is the JSON structure from PreCompact hook.
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

// subagentStartHookInputRaw is the JSON structure from SubagentStart hook.
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

// subagentStopHookInputRaw is the JSON structure from SubagentStop hooks.
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
