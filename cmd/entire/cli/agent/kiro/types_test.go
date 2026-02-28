package kiro

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// --- hookInputRaw JSON parsing ---

func TestHookInputRaw_AgentSpawnPayload(t *testing.T) {
	t.Parallel()

	// Realistic payload from AGENT.md: agentSpawn event
	input := `{
		"hook_event_name": "agentSpawn",
		"cwd": "/home/user/project"
	}`

	result, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HookEventName != "agentSpawn" {
		t.Errorf("HookEventName = %q, want %q", result.HookEventName, "agentSpawn")
	}
	if result.CWD != "/home/user/project" {
		t.Errorf("CWD = %q, want %q", result.CWD, "/home/user/project")
	}
	if result.Prompt != "" {
		t.Errorf("Prompt = %q, want empty", result.Prompt)
	}
	if result.ToolName != "" {
		t.Errorf("ToolName = %q, want empty", result.ToolName)
	}
}

func TestHookInputRaw_UserPromptSubmitPayload(t *testing.T) {
	t.Parallel()

	// Realistic payload from AGENT.md: userPromptSubmit event with prompt
	input := `{
		"hook_event_name": "userPromptSubmit",
		"cwd": "/home/user/project",
		"prompt": "Create a new file called hello.txt with the content Hello World"
	}`

	result, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HookEventName != "userPromptSubmit" {
		t.Errorf("HookEventName = %q, want %q", result.HookEventName, "userPromptSubmit")
	}
	if result.CWD != "/home/user/project" {
		t.Errorf("CWD = %q, want %q", result.CWD, "/home/user/project")
	}
	if result.Prompt != "Create a new file called hello.txt with the content Hello World" {
		t.Errorf("Prompt = %q, want the full prompt text", result.Prompt)
	}
}

func TestHookInputRaw_PreToolUsePayload(t *testing.T) {
	t.Parallel()

	// Realistic payload: preToolUse event with tool fields
	input := `{
		"hook_event_name": "preToolUse",
		"cwd": "/home/user/project",
		"tool_name": "fs_write",
		"tool_input": "{\"path\":\"hello.txt\",\"content\":\"Hello World\"}"
	}`

	result, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HookEventName != "preToolUse" {
		t.Errorf("HookEventName = %q, want %q", result.HookEventName, "preToolUse")
	}
	if result.ToolName != "fs_write" {
		t.Errorf("ToolName = %q, want %q", result.ToolName, "fs_write")
	}
	if result.ToolInput == "" {
		t.Error("ToolInput should not be empty for preToolUse")
	}
	if result.Prompt != "" {
		t.Errorf("Prompt = %q, want empty for preToolUse", result.Prompt)
	}
}

func TestHookInputRaw_PostToolUsePayload(t *testing.T) {
	t.Parallel()

	// Realistic payload: postToolUse event with tool_response
	input := `{
		"hook_event_name": "postToolUse",
		"cwd": "/home/user/project",
		"tool_name": "fs_write",
		"tool_input": "{\"path\":\"hello.txt\",\"content\":\"Hello World\"}",
		"tool_response": "File written successfully"
	}`

	result, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HookEventName != "postToolUse" {
		t.Errorf("HookEventName = %q, want %q", result.HookEventName, "postToolUse")
	}
	if result.ToolName != "fs_write" {
		t.Errorf("ToolName = %q, want %q", result.ToolName, "fs_write")
	}
	if result.ToolResponse != "File written successfully" {
		t.Errorf("ToolResponse = %q, want %q", result.ToolResponse, "File written successfully")
	}
}

func TestHookInputRaw_StopPayload(t *testing.T) {
	t.Parallel()

	// Realistic payload: stop event
	input := `{
		"hook_event_name": "stop",
		"cwd": "/workspace/myapp"
	}`

	result, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HookEventName != "stop" {
		t.Errorf("HookEventName = %q, want %q", result.HookEventName, "stop")
	}
	if result.CWD != "/workspace/myapp" {
		t.Errorf("CWD = %q, want %q", result.CWD, "/workspace/myapp")
	}
}

func TestHookInputRaw_PartialPayload(t *testing.T) {
	t.Parallel()

	// Only hook_event_name present; other fields should be zero values
	input := `{"hook_event_name": "stop"}`

	result, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HookEventName != "stop" {
		t.Errorf("HookEventName = %q, want %q", result.HookEventName, "stop")
	}
	if result.CWD != "" {
		t.Errorf("CWD = %q, want empty", result.CWD)
	}
}

func TestHookInputRaw_ExtraFieldsIgnored(t *testing.T) {
	t.Parallel()

	// JSON with extra unknown fields should be parsed without error
	input := `{
		"hook_event_name": "userPromptSubmit",
		"cwd": "/tmp",
		"prompt": "hello",
		"extra_field": "should be ignored",
		"another_unknown": 42
	}`

	result, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HookEventName != "userPromptSubmit" {
		t.Errorf("HookEventName = %q, want %q", result.HookEventName, "userPromptSubmit")
	}
	if result.Prompt != "hello" {
		t.Errorf("Prompt = %q, want %q", result.Prompt, "hello")
	}
}

func TestHookInputRaw_EmptyInput(t *testing.T) {
	t.Parallel()

	_, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
	if !strings.Contains(err.Error(), "empty hook input") {
		t.Errorf("expected 'empty hook input' error, got: %v", err)
	}
}

func TestHookInputRaw_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader("not valid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse hook input") {
		t.Errorf("expected 'failed to parse hook input' error, got: %v", err)
	}
}

// --- kiroAgentFile JSON structure ---

func TestKiroAgentFile_MarshalRoundTrip(t *testing.T) {
	t.Parallel()

	file := kiroAgentFile{
		Name:  "entire",
		Tools: []string{"read", "write", "shell"},
		Hooks: kiroHooks{
			AgentSpawn:       []kiroHookEntry{{Command: "entire hooks kiro agent-spawn"}},
			UserPromptSubmit: []kiroHookEntry{{Command: "entire hooks kiro user-prompt-submit"}},
			PreToolUse:       []kiroHookEntry{{Command: "entire hooks kiro pre-tool-use"}},
			PostToolUse:      []kiroHookEntry{{Command: "entire hooks kiro post-tool-use"}},
			Stop:             []kiroHookEntry{{Command: "entire hooks kiro stop"}},
		},
	}

	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("failed to marshal kiroAgentFile: %v", err)
	}

	var result kiroAgentFile
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal kiroAgentFile: %v", err)
	}

	if result.Name != file.Name {
		t.Errorf("Name = %q, want %q", result.Name, file.Name)
	}
	if len(result.Tools) != len(file.Tools) {
		t.Errorf("Tools length = %d, want %d", len(result.Tools), len(file.Tools))
	}
	if len(result.Hooks.AgentSpawn) != 1 {
		t.Errorf("AgentSpawn hooks = %d, want 1", len(result.Hooks.AgentSpawn))
	}
	if len(result.Hooks.UserPromptSubmit) != 1 {
		t.Errorf("UserPromptSubmit hooks = %d, want 1", len(result.Hooks.UserPromptSubmit))
	}
	if len(result.Hooks.Stop) != 1 {
		t.Errorf("Stop hooks = %d, want 1", len(result.Hooks.Stop))
	}
}

func TestKiroAgentFile_UnmarshalFromAGENTMD(t *testing.T) {
	t.Parallel()

	// This is the exact JSON structure from AGENT.md
	configJSON := `{
		"name": "entire",
		"tools": ["read", "write", "shell", "grep", "glob", "aws", "report",
				  "introspect", "knowledge", "thinking", "todo", "delegate"],
		"hooks": {
			"agentSpawn": [{"command": "entire hooks kiro agent-spawn"}],
			"userPromptSubmit": [{"command": "entire hooks kiro user-prompt-submit"}],
			"preToolUse": [{"command": "entire hooks kiro pre-tool-use"}],
			"postToolUse": [{"command": "entire hooks kiro post-tool-use"}],
			"stop": [{"command": "entire hooks kiro stop"}]
		}
	}`

	var file kiroAgentFile
	if err := json.Unmarshal([]byte(configJSON), &file); err != nil {
		t.Fatalf("failed to unmarshal AGENT.md config: %v", err)
	}

	if file.Name != "entire" {
		t.Errorf("Name = %q, want %q", file.Name, "entire")
	}
	if len(file.Tools) != 12 {
		t.Errorf("Tools length = %d, want 12", len(file.Tools))
	}
	if len(file.Hooks.AgentSpawn) != 1 {
		t.Errorf("AgentSpawn hooks = %d, want 1", len(file.Hooks.AgentSpawn))
	}
	if file.Hooks.AgentSpawn[0].Command != "entire hooks kiro agent-spawn" {
		t.Errorf("AgentSpawn command = %q, want %q",
			file.Hooks.AgentSpawn[0].Command, "entire hooks kiro agent-spawn")
	}
	if len(file.Hooks.UserPromptSubmit) != 1 {
		t.Errorf("UserPromptSubmit hooks = %d, want 1", len(file.Hooks.UserPromptSubmit))
	}
	if file.Hooks.UserPromptSubmit[0].Command != "entire hooks kiro user-prompt-submit" {
		t.Errorf("UserPromptSubmit command = %q, want %q",
			file.Hooks.UserPromptSubmit[0].Command, "entire hooks kiro user-prompt-submit")
	}
	if len(file.Hooks.Stop) != 1 {
		t.Errorf("Stop hooks = %d, want 1", len(file.Hooks.Stop))
	}
	if file.Hooks.Stop[0].Command != "entire hooks kiro stop" {
		t.Errorf("Stop command = %q, want %q",
			file.Hooks.Stop[0].Command, "entire hooks kiro stop")
	}
}

func TestKiroHooks_OmitEmpty(t *testing.T) {
	t.Parallel()

	// Empty hooks should omit fields from JSON
	hooks := kiroHooks{}
	data, err := json.Marshal(hooks)
	if err != nil {
		t.Fatalf("failed to marshal empty hooks: %v", err)
	}

	// Should be an empty JSON object since all fields are omitempty
	if string(data) != "{}" {
		t.Errorf("empty kiroHooks marshaled to %s, want {}", string(data))
	}
}

func TestKiroHookEntry_Marshal(t *testing.T) {
	t.Parallel()

	entry := kiroHookEntry{Command: "entire hooks kiro stop"}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal entry: %v", err)
	}
	if string(data) != `{"command":"entire hooks kiro stop"}` {
		t.Errorf("entry marshaled to %s, want %s", string(data), `{"command":"entire hooks kiro stop"}`)
	}
}

// --- Table-driven: all hook event payloads ---

func TestHookInputRaw_AllEventTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		input         string
		wantEvent     string
		wantCWD       string
		wantPrompt    string
		wantToolName  string
		wantToolInput string
	}{
		{
			name:      "agentSpawn",
			input:     `{"hook_event_name":"agentSpawn","cwd":"/repo"}`,
			wantEvent: "agentSpawn",
			wantCWD:   "/repo",
		},
		{
			name:       "userPromptSubmit",
			input:      `{"hook_event_name":"userPromptSubmit","cwd":"/repo","prompt":"fix bug"}`,
			wantEvent:  "userPromptSubmit",
			wantCWD:    "/repo",
			wantPrompt: "fix bug",
		},
		{
			name:          "preToolUse",
			input:         `{"hook_event_name":"preToolUse","cwd":"/repo","tool_name":"fs_write","tool_input":"{}"}`,
			wantEvent:     "preToolUse",
			wantCWD:       "/repo",
			wantToolName:  "fs_write",
			wantToolInput: "{}",
		},
		{
			name:      "stop",
			input:     `{"hook_event_name":"stop","cwd":"/repo"}`,
			wantEvent: "stop",
			wantCWD:   "/repo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := agent.ReadAndParseHookInput[hookInputRaw](strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.HookEventName != tc.wantEvent {
				t.Errorf("HookEventName = %q, want %q", result.HookEventName, tc.wantEvent)
			}
			if result.CWD != tc.wantCWD {
				t.Errorf("CWD = %q, want %q", result.CWD, tc.wantCWD)
			}
			if result.Prompt != tc.wantPrompt {
				t.Errorf("Prompt = %q, want %q", result.Prompt, tc.wantPrompt)
			}
			if result.ToolName != tc.wantToolName {
				t.Errorf("ToolName = %q, want %q", result.ToolName, tc.wantToolName)
			}
			if result.ToolInput != tc.wantToolInput {
				t.Errorf("ToolInput = %q, want %q", result.ToolInput, tc.wantToolInput)
			}
		})
	}
}
