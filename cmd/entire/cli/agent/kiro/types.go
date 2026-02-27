package kiro

// hookInputRaw matches the JSON payload piped from Kiro hooks on stdin.
// All hooks share the same structure; fields are populated based on the hook event.
type hookInputRaw struct {
	HookEventName string `json:"hook_event_name"`
	CWD           string `json:"cwd"`
	Prompt        string `json:"prompt,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ToolInput     string `json:"tool_input,omitempty"`
	ToolResponse  string `json:"tool_response,omitempty"`
}

// --- Kiro conversation JSON types (from SQLite `conversations_v2.value` column) ---

// Conversation represents the JSON blob stored in Kiro's SQLite database.
type Conversation struct {
	ConversationID string         `json:"conversation_id"`
	History        []HistoryEntry `json:"history"`
}

// HistoryEntry represents a single turn in a Kiro conversation.
// The Role field distinguishes user messages, assistant responses, and metadata.
type HistoryEntry struct {
	Role    string        `json:"role"` // "user", "assistant", "request_metadata"
	Content []ContentPart `json:"content,omitempty"`

	// request_metadata fields (token usage)
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	CacheRead    int `json:"cache_read,omitempty"`
	CacheWrite   int `json:"cache_write,omitempty"`
}

// ContentPart represents a part of a message content array.
type ContentPart struct {
	Type string `json:"type"` // "text", "tool_use", "tool_result"

	// Text content
	Text string `json:"text,omitempty"`

	// Tool use fields
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`

	// Tool result fields
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

// Message role constants.
const (
	roleUser            = "user"
	roleAssistant       = "assistant"
	roleRequestMetadata = "request_metadata"
)

// FileModificationTools are tools in Kiro that modify files on disk.
// These match the tool names Kiro uses for file operations.
var FileModificationTools = []string{
	"fs_write",
	"str_replace",
	"create_file",
	"write_file",
	"edit_file",
}
