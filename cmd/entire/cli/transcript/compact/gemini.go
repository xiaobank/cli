package compact

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/entireio/cli/cmd/entire/cli/transcript"
)

// --- Gemini CLI format support ---
//
// Gemini transcripts are a single JSON object (not JSONL):
//
//	{"sessionId":"...","messages":[{"id":"...","timestamp":"...","type":"user"|"gemini"|"info","content":"...","toolCalls":[...],"thoughts":[...]}]}
//
// Key differences from other formats:
//   - Assistant messages use type "gemini" (not "assistant")
//   - System messages use type "info" and should be dropped
//   - Tool calls are at the message level in a "toolCalls" array
//   - A message can have both content text and toolCalls
//   - Timestamps are ISO strings (not millisecond integers)
//   - "thoughts" and "tokens" are metadata fields that should be stripped

// geminiDroppedTypes are Gemini message types that carry no transcript-relevant data.
var geminiDroppedTypes = map[string]bool{
	"info": true,
}

// isGeminiFormat checks whether content is a single JSON object with the
// Gemini session shape: top-level "sessionId" and "messages" keys, but NO "info"
// key (which distinguishes it from OpenCode format).
func isGeminiFormat(content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var probe struct {
		SessionID *json.RawMessage `json:"sessionId"`
		Messages  *json.RawMessage `json:"messages"`
		Info      *json.RawMessage `json:"info"`
	}
	if json.Unmarshal(trimmed, &probe) != nil {
		return false
	}
	return probe.SessionID != nil && probe.Messages != nil && probe.Info == nil
}

// geminiMessage mirrors the Gemini message structure for unmarshaling.
type geminiMessage struct {
	ID        string           `json:"id"`
	Timestamp string           `json:"timestamp"`
	Type      string           `json:"type"`
	Content   string           `json:"content"`
	ToolCalls []geminiToolCall `json:"toolCalls"`
	Thoughts  json.RawMessage  `json:"thoughts"` // parsed only to detect; always dropped
	Tokens    json.RawMessage  `json:"tokens"`   // dropped
	Model     string           `json:"model"`    // dropped
}

// geminiToolCall represents a single tool invocation within a Gemini message.
type geminiToolCall struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Args   map[string]interface{} `json:"args"`
	Result []geminiToolResult     `json:"result"`
	Status string                 `json:"status"`
}

// geminiToolResult represents a tool result entry from the Gemini format.
type geminiToolResult struct {
	FunctionResponse struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Response struct {
			Output string `json:"output"`
		} `json:"response"`
	} `json:"functionResponse"`
}

// compactGemini converts a full Gemini session JSON into transcript lines.
func compactGemini(content []byte, opts Options) ([]byte, error) {
	var session struct {
		Messages []geminiMessage `json:"messages"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(content), &session); err != nil {
		return nil, fmt.Errorf("parsing gemini session: %w", err)
	}

	meta := newCompactMeta(opts)
	var result []byte

	for _, msg := range session.Messages {
		if geminiDroppedTypes[msg.Type] {
			continue
		}

		ts := mustMarshal(msg.Timestamp)

		switch msg.Type {
		case transcript.TypeUser:
			line := convertGeminiUser(msg, ts, meta)
			if line != nil {
				result = append(result, line...)
				result = append(result, '\n')
			}
		case "gemini":
			line := convertGeminiAssistant(msg, ts, meta)
			if line != nil {
				result = append(result, line...)
				result = append(result, '\n')
			}
		}
	}

	return result, nil
}

// convertGeminiUser produces a single user line. Gemini user messages have
// plain string content.
func convertGeminiUser(msg geminiMessage, ts json.RawMessage, meta compactMeta) []byte {
	if msg.Content == "" {
		return nil
	}

	return marshalOrdered(
		"v", meta.v,
		"agent", meta.agent,
		"cli_version", meta.cliVersion,
		"type", mustMarshal(transcript.TypeUser),
		"ts", ts,
		"id", mustMarshal(msg.ID),
		"content", mustMarshal(msg.Content),
	)
}

// convertGeminiAssistant produces a single assistant line. The content array
// contains text blocks (from content field) and tool_use blocks (from toolCalls).
func convertGeminiAssistant(msg geminiMessage, ts json.RawMessage, meta compactMeta) []byte {
	contentBlocks := make([]map[string]json.RawMessage, 0, 1+len(msg.ToolCalls))

	// Add text block if content is non-empty.
	if msg.Content != "" {
		contentBlocks = append(contentBlocks, map[string]json.RawMessage{
			"type": mustMarshal(transcript.ContentTypeText),
			"text": mustMarshal(msg.Content),
		})
	}

	// Add tool_use blocks from toolCalls.
	for _, tc := range msg.ToolCalls {
		toolBlock := map[string]json.RawMessage{
			"type":  mustMarshal(transcript.ContentTypeToolUse),
			"id":    mustMarshal(tc.ID),
			"name":  mustMarshal(tc.Name),
			"input": mustMarshal(tc.Args),
		}

		toolBlock["result"] = geminiToolResultCompact(tc)
		contentBlocks = append(contentBlocks, toolBlock)
	}

	if len(contentBlocks) == 0 {
		return nil
	}

	contentJSON, err := json.Marshal(contentBlocks)
	if err != nil {
		return nil
	}

	return marshalOrdered(
		"v", meta.v,
		"agent", meta.agent,
		"cli_version", meta.cliVersion,
		"type", mustMarshal(transcript.TypeAssistant),
		"ts", ts,
		"id", mustMarshal(msg.ID),
		"content", json.RawMessage(contentJSON),
	)
}

// geminiToolResultCompact builds the compact {"output":"...","status":"..."}
// object from a Gemini tool call.
func geminiToolResultCompact(tc geminiToolCall) json.RawMessage {
	output := ""
	if len(tc.Result) > 0 {
		output = tc.Result[0].FunctionResponse.Response.Output
	}

	result := map[string]string{
		"output": output,
		"status": tc.Status,
	}
	return mustMarshal(result)
}
