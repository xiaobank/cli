package cursor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// HookNames returns the hook verbs Cursor supports.
// Delegates to GetHookNames for backward compatibility.
func (c *CursorAgent) HookNames() []string {
	return c.GetHookNames()
}

// ParseHookEvent translates a Cursor hook into a normalized lifecycle Event.
// Returns nil if the hook has no lifecycle significance.
func (c *CursorAgent) ParseHookEvent(hookName string, stdin io.Reader) (*agent.Event, error) {
	switch hookName {
	case HookNameSessionStart:
		return c.parseSessionStart(stdin)
	case HookNameBeforeSubmitPrompt:
		return c.parseTurnStart(stdin)
	case HookNameStop:
		return c.parseTurnEnd(stdin)
	case HookNameSessionEnd:
		return c.parseSessionEnd(stdin)
	case HookNamePreTask:
		return c.parseSubagentStart(stdin)
	case HookNamePostTask:
		return c.parseSubagentEnd(stdin)
	case HookNamePostTodo:
		// PostTodo is handled outside the generic dispatcher (incremental checkpoints).
		return nil, nil //nolint:nilnil // nil event = no lifecycle action
	default:
		return nil, nil //nolint:nilnil // Unknown hooks have no lifecycle action
	}
}

// ReadTranscript reads the raw JSONL transcript bytes for a session.
func (c *CursorAgent) ReadTranscript(sessionRef string) ([]byte, error) {
	data, err := os.ReadFile(sessionRef) //nolint:gosec // Path comes from agent hook input
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript: %w", err)
	}
	return data, nil
}

// --- Internal hook parsing functions ---

func (c *CursorAgent) parseSessionStart(stdin io.Reader) (*agent.Event, error) {
	raw, err := readAndParse[sessionInfoRaw](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.SessionStart,
		SessionID:  raw.getSessionID(),
		SessionRef: raw.TranscriptPath,
		Timestamp:  time.Now(),
	}, nil
}

func (c *CursorAgent) parseTurnStart(stdin io.Reader) (*agent.Event, error) {
	raw, err := readAndParse[userPromptSubmitRaw](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.TurnStart,
		SessionID:  raw.getSessionID(),
		SessionRef: raw.TranscriptPath,
		Prompt:     raw.Prompt,
		Timestamp:  time.Now(),
	}, nil
}

func (c *CursorAgent) parseTurnEnd(stdin io.Reader) (*agent.Event, error) {
	raw, err := readAndParse[sessionInfoRaw](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.TurnEnd,
		SessionID:  raw.getSessionID(),
		SessionRef: raw.TranscriptPath,
		Timestamp:  time.Now(),
	}, nil
}

func (c *CursorAgent) parseSessionEnd(stdin io.Reader) (*agent.Event, error) {
	raw, err := readAndParse[sessionInfoRaw](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.SessionEnd,
		SessionID:  raw.getSessionID(),
		SessionRef: raw.TranscriptPath,
		Timestamp:  time.Now(),
	}, nil
}

func (c *CursorAgent) parseSubagentStart(stdin io.Reader) (*agent.Event, error) {
	raw, err := readAndParse[taskHookInputRaw](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.SubagentStart,
		SessionID:  raw.getSessionID(),
		SessionRef: raw.TranscriptPath,
		ToolUseID:  raw.ToolUseID,
		ToolInput:  raw.ToolInput,
		Timestamp:  time.Now(),
	}, nil
}

func (c *CursorAgent) parseSubagentEnd(stdin io.Reader) (*agent.Event, error) {
	raw, err := readAndParse[postToolHookInputRaw](stdin)
	if err != nil {
		return nil, err
	}
	event := &agent.Event{
		Type:       agent.SubagentEnd,
		SessionID:  raw.getSessionID(),
		SessionRef: raw.TranscriptPath,
		ToolUseID:  raw.ToolUseID,
		ToolInput:  raw.ToolInput,
		Timestamp:  time.Now(),
	}
	if raw.ToolResponse.AgentID != "" {
		event.SubagentID = raw.ToolResponse.AgentID
	}
	return event, nil
}

// readAndParse reads stdin and unmarshals JSON into the given type.
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
