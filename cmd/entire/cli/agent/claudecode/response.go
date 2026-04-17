package claudecode

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type responseEnvelope struct {
	Type   string  `json:"type"`
	Result *string `json:"result"`
}

// parseGenerateTextResponse extracts the raw text payload from Claude CLI JSON output.
// Claude may return either a legacy single object or a newer array of events.
func parseGenerateTextResponse(stdout []byte) (string, error) {
	var response responseEnvelope
	if err := json.Unmarshal(stdout, &response); err == nil && response.Result != nil {
		return *response.Result, nil
	}

	var responses []responseEnvelope
	if err := json.Unmarshal(stdout, &responses); err != nil {
		return "", fmt.Errorf("unsupported Claude CLI JSON response: %w", err)
	}

	for i := len(responses) - 1; i >= 0; i-- {
		if responses[i].Type == "result" && responses[i].Result != nil {
			return *responses[i].Result, nil
		}
	}

	return "", errors.New("unsupported Claude CLI JSON response: missing result item")
}

// streamBufferMax bounds a single NDJSON line. Stream events can be large
// (init carries the full tool list; deltas can carry long thinking chunks)
// so we lift the default scanner limit substantially.
const streamBufferMax = 4 * 1024 * 1024 // 4 MiB

// StreamEvent represents one decoded line from the stream-json NDJSON output.
// Fields are populated based on the event Type/Subtype.
type StreamEvent struct {
	Type    string           `json:"type"`
	Subtype string           `json:"subtype"`
	Status  string           `json:"status"` // e.g. "requesting" for type=system,subtype=status
	Event   StreamInnerEvent `json:"event"`  // for type=stream_event

	// Fields populated for type=result.
	IsError        bool          `json:"is_error"`
	APIErrorStatus *int          `json:"api_error_status"`
	Result         *string       `json:"result"`
	DurationMs     int           `json:"duration_ms"`
	TTFTms         int           `json:"ttft_ms,omitempty"` // time-to-first-token; on outer stream_event envelope
	Usage          *messageUsage `json:"usage"`
}

// StreamInnerEvent holds the nested "event" payload for type=stream_event.
type StreamInnerEvent struct {
	Type    string         `json:"type"` // "message_start" | "content_block_delta" | "message_delta" | ...
	Delta   *StreamDelta   `json:"delta,omitempty"`
	Message *StreamMessage `json:"message,omitempty"`
}

// StreamDelta carries the content-block delta payload.
type StreamDelta struct {
	Type     string `json:"type"` // "text_delta" | "thinking_delta" | ...
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

// StreamMessage is the partial-message payload on message_start. Only the
// usage field is currently consumed by callers; other fields are ignored.
type StreamMessage struct {
	Usage *messageUsage `json:"usage,omitempty"`
}

// streamClaudeResponse reads NDJSON-encoded events from r, invokes onEvent
// for every successfully decoded event, and returns the final result event
// once the stream ends. Malformed lines are skipped silently to keep the
// stream resilient against single-line corruption. If the stream ends
// without ever producing a result event, returns (nil, error).
func streamClaudeResponse(r io.Reader, onEvent func(StreamEvent)) (*StreamEvent, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), streamBufferMax)
	var final *StreamEvent
	var malformedLines int
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev StreamEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			malformedLines++
			continue // best-effort: skip and keep streaming
		}
		if onEvent != nil {
			onEvent(ev)
		}
		if ev.Type == "result" {
			captured := ev
			final = &captured
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading claude stream: %w", err)
	}
	if final == nil {
		if malformedLines > 0 {
			return nil, fmt.Errorf("claude stream ended without a result event (%d malformed lines skipped)", malformedLines)
		}
		return nil, errors.New("claude stream ended without a result event")
	}
	return final, nil
}
