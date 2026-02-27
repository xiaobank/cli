package kiro

import (
	"fmt"
	"os"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// Compile-time interface assertions
var (
	_ agent.TranscriptAnalyzer = (*KiroAgent)(nil)
	_ agent.TranscriptPreparer = (*KiroAgent)(nil)
	_ agent.TokenCalculator    = (*KiroAgent)(nil)
)

// parseConversationFromFile reads a file and parses its contents as a Conversation.
func parseConversationFromFile(path string) (*Conversation, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path from agent hook/session state
	if err != nil {
		return nil, err //nolint:wrapcheck // caller adds context or checks os.IsNotExist
	}
	return ParseConversation(data)
}

// GetTranscriptPosition returns the number of history entries in the transcript.
func (k *KiroAgent) GetTranscriptPosition(path string) (int, error) {
	conv, err := parseConversationFromFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if conv == nil {
		return 0, nil
	}
	return len(conv.History), nil
}

// ExtractModifiedFilesFromOffset extracts files modified by tool calls from the given history offset.
func (k *KiroAgent) ExtractModifiedFilesFromOffset(path string, startOffset int) ([]string, int, error) {
	conv, err := parseConversationFromFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	if conv == nil {
		return nil, 0, nil
	}

	seen := make(map[string]bool)
	var files []string

	for i := startOffset; i < len(conv.History); i++ {
		entry := conv.History[i]
		if entry.Role != roleAssistant {
			continue
		}
		for _, part := range entry.Content {
			if part.Type != "tool_use" {
				continue
			}
			if !isFileModificationTool(part.Name) {
				continue
			}
			for _, filePath := range extractFilePathsFromInput(part.Input) {
				if !seen[filePath] {
					seen[filePath] = true
					files = append(files, filePath)
				}
			}
		}
	}

	return files, len(conv.History), nil
}

// ExtractPrompts extracts user prompt strings from the transcript starting at the given offset.
func (k *KiroAgent) ExtractPrompts(sessionRef string, fromOffset int) ([]string, error) {
	conv, err := parseConversationFromFile(sessionRef)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if conv == nil {
		return nil, nil
	}

	var prompts []string
	for i := fromOffset; i < len(conv.History); i++ {
		entry := conv.History[i]
		if entry.Role != roleUser {
			continue
		}
		content := extractTextFromContent(entry.Content)
		if content != "" {
			prompts = append(prompts, content)
		}
	}

	return prompts, nil
}

// ExtractSummary extracts the last assistant message content as a summary.
func (k *KiroAgent) ExtractSummary(sessionRef string) (string, error) {
	conv, err := parseConversationFromFile(sessionRef)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if conv == nil {
		return "", nil
	}

	for i := len(conv.History) - 1; i >= 0; i-- {
		entry := conv.History[i]
		if entry.Role == roleAssistant {
			content := extractTextFromContent(entry.Content)
			if content != "" {
				return content, nil
			}
		}
	}

	return "", nil
}

// CalculateTokenUsage computes token usage from request_metadata entries starting at the given offset.
func (k *KiroAgent) CalculateTokenUsage(transcriptData []byte, fromOffset int) (*agent.TokenUsage, error) {
	conv, err := ParseConversation(transcriptData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transcript for token usage: %w", err)
	}
	if conv == nil {
		return nil, nil //nolint:nilnil // nil usage for empty data is expected
	}

	usage := &agent.TokenUsage{}
	for i := fromOffset; i < len(conv.History); i++ {
		entry := conv.History[i]
		if entry.Role != roleRequestMetadata {
			continue
		}
		usage.InputTokens += entry.InputTokens
		usage.OutputTokens += entry.OutputTokens
		usage.CacheReadTokens += entry.CacheRead
		usage.CacheCreationTokens += entry.CacheWrite
		usage.APICallCount++
	}

	return usage, nil
}

// ExtractAllUserPrompts extracts all user prompts from raw conversation JSON bytes.
func ExtractAllUserPrompts(data []byte) ([]string, error) {
	conv, err := ParseConversation(data)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, nil
	}

	var prompts []string
	for _, entry := range conv.History {
		if entry.Role != roleUser {
			continue
		}
		content := extractTextFromContent(entry.Content)
		if content != "" {
			prompts = append(prompts, content)
		}
	}
	return prompts, nil
}

// extractTextFromContent extracts text content from content parts.
func extractTextFromContent(parts []ContentPart) string {
	var texts []string
	for _, part := range parts {
		if part.Type == "text" && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}
