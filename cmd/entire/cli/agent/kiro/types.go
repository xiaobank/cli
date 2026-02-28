package kiro

// hookInputRaw matches Kiro's hook stdin JSON payload.
// All hooks receive the same structure; fields are populated based on the event.
type hookInputRaw struct {
	HookEventName string `json:"hook_event_name"`
	CWD           string `json:"cwd"`
	Prompt        string `json:"prompt,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ToolInput     string `json:"tool_input,omitempty"`
	ToolResponse  string `json:"tool_response,omitempty"`
}

// kiroAgentFile represents the .kiro/agents/entire.json structure.
// This is a Kiro agent definition file — hooks are nested under the "hooks" field.
// Entire owns this file entirely — no round-trip preservation needed.
type kiroAgentFile struct {
	Name  string    `json:"name"`
	Tools []string  `json:"tools"`
	Hooks kiroHooks `json:"hooks"`
}

// kiroHooks contains all hook configurations using camelCase keys.
type kiroHooks struct {
	AgentSpawn       []kiroHookEntry `json:"agentSpawn,omitempty"`
	UserPromptSubmit []kiroHookEntry `json:"userPromptSubmit,omitempty"`
	PreToolUse       []kiroHookEntry `json:"preToolUse,omitempty"`
	PostToolUse      []kiroHookEntry `json:"postToolUse,omitempty"`
	Stop             []kiroHookEntry `json:"stop,omitempty"`
}

// kiroHookEntry represents a single hook command in the config file.
type kiroHookEntry struct {
	Command string `json:"command"`
}
