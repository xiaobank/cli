package compact

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/entireio/cli/cmd/entire/cli/textutil"
	"github.com/entireio/cli/cmd/entire/cli/transcript"
)

// Copilot CLI uses events.jsonl with event types like "user.message" and
// "assistant.message". This is distinct from Claude/Cursor JSONL shapes.
var copilotEventTypes = map[string]bool{
	"user.message":            true,
	"assistant.message":       true,
	"assistant.turn_start":    true,
	"assistant.turn_end":      true,
	"tool.execution_complete": true,
	"session.start":           true,
	"session.shutdown":        true,
	"session.model_change":    true,
}

// isCopilotFormat checks whether JSONL content looks like Copilot CLI events.
func isCopilotFormat(content []byte) bool {
	reader := bufio.NewReader(bytes.NewReader(content))
	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return false
		}

		line := bytes.TrimSpace(lineBytes)
		if len(line) > 0 {
			var raw map[string]json.RawMessage
			if json.Unmarshal(line, &raw) == nil {
				eventType := unquote(raw["type"])
				if copilotEventTypes[eventType] {
					return true
				}
				// If the first parseable non-empty line has a conventional JSONL
				// transcript type ("user"/"assistant"), this is not Copilot.
				if userAliases[eventType] || eventType == transcript.TypeAssistant {
					return false
				}
			}
		}

		if err == io.EOF {
			break
		}
	}
	return false
}

func compactCopilot(content []byte, opts Options) ([]byte, error) {
	reader := bufio.NewReader(bytes.NewReader(content))
	meta := newCompactMeta(opts)
	var result []byte

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("reading copilot jsonl line: %w", err)
		}

		line := bytes.TrimSpace(lineBytes)
		if len(line) > 0 {
			b := compactCopilotLine(line, meta)
			if b != nil {
				result = append(result, b...)
				result = append(result, '\n')
			}
		}

		if err == io.EOF {
			break
		}
	}

	return result, nil
}

func compactCopilotLine(line []byte, meta compactMeta) []byte {
	var raw map[string]json.RawMessage
	if json.Unmarshal(line, &raw) != nil {
		return nil
	}

	eventType := unquote(raw["type"])
	ts := raw["timestamp"]
	id := raw["id"]

	dataRaw, ok := raw["data"]
	if !ok {
		return nil
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(dataRaw, &data) != nil {
		return nil
	}

	content := unquote(data["content"])
	if content == "" {
		return nil
	}

	switch eventType {
	case "user.message":
		return marshalOrdered(
			"v", meta.v,
			"agent", meta.agent,
			"cli_version", meta.cliVersion,
			"type", mustMarshal(transcript.TypeUser),
			"ts", ts,
			"content", mustMarshal(textutil.StripIDEContextTags(content)),
		)
	case "assistant.message":
		return marshalOrdered(
			"v", meta.v,
			"agent", meta.agent,
			"cli_version", meta.cliVersion,
			"type", mustMarshal(transcript.TypeAssistant),
			"ts", ts,
			"id", id,
			"content", mustMarshal(content),
		)
	default:
		return nil
	}
}
