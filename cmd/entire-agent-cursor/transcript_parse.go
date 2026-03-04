package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Transcript type constants.
const (
	typeUser        = "user"
	typeAssistant   = "assistant"
	contentTypeText = "text"
)

// line represents a single line in a Cursor JSONL transcript.
type line struct {
	Type    string          `json:"type"`
	Role    string          `json:"role,omitempty"`
	UUID    string          `json:"uuid"`
	Message json.RawMessage `json:"message"`
}

// userMessage represents a user message in the transcript.
type userMessage struct {
	Content any `json:"content"`
}

// assistantMessage represents an assistant message in the transcript.
type assistantMessage struct {
	Content []contentBlock `json:"content"`
}

// contentBlock represents a block within an assistant message.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// normalizeLineType ensures line.Type is populated.
// Cursor uses "role" while Claude Code uses "type".
func normalizeLineType(l *line) {
	if l.Type == "" && l.Role != "" {
		l.Type = l.Role
	}
}

// parseFromBytes parses transcript content from a byte slice.
func parseFromBytes(content []byte) ([]line, error) {
	var lines []line
	reader := bufio.NewReader(bytes.NewReader(content))

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read transcript: %w", err)
		}

		if len(lineBytes) == 0 {
			if err == io.EOF {
				break
			}
			continue
		}

		var l line
		if err := json.Unmarshal(lineBytes, &l); err == nil {
			normalizeLineType(&l)
			lines = append(lines, l)
		}

		if err == io.EOF {
			break
		}
	}

	return lines, nil
}

// parseFromFileAtLine reads and parses a transcript file starting from a specific line.
func parseFromFileAtLine(path string, startLine int) ([]line, error) {
	file, err := os.Open(path) //nolint:gosec // path is a controlled transcript file path
	if err != nil {
		return nil, fmt.Errorf("failed to open transcript: %w", err)
	}
	defer func() { _ = file.Close() }()

	var lines []line
	reader := bufio.NewReader(file)

	totalLines := 0
	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read transcript: %w", err)
		}

		if len(lineBytes) == 0 {
			if err == io.EOF {
				break
			}
			continue
		}

		if totalLines >= startLine {
			var l line
			if err := json.Unmarshal(lineBytes, &l); err == nil {
				normalizeLineType(&l)
				lines = append(lines, l)
			}
		}
		totalLines++

		if err == io.EOF {
			break
		}
	}

	return lines, nil
}

// extractUserContent extracts user content from a raw message.
// Handles both string and array content formats.
func extractUserContent(message json.RawMessage) string {
	var msg userMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return ""
	}

	// Handle string content
	if str, ok := msg.Content.(string); ok {
		return stripIDEContextTags(str)
	}

	// Handle array content
	if arr, ok := msg.Content.([]any); ok {
		var texts []string
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == contentTypeText {
					if text, ok := m["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		if len(texts) > 0 {
			return stripIDEContextTags(strings.Join(texts, "\n\n"))
		}
	}

	return ""
}
