package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/textutil"
	"github.com/entireio/cli/cmd/entire/cli/transcript"
)

// cmdChunkTranscript splits JSONL content from stdin at line boundaries.
func cmdChunkTranscript(maxSize int) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}

	chunks, err := agent.ChunkJSONL(data, maxSize)
	if err != nil {
		return fmt.Errorf("failed to chunk JSONL transcript: %w", err)
	}

	return writeJSON(struct {
		Chunks [][]byte `json:"chunks"`
	}{Chunks: chunks})
}

// cmdReassembleTranscript concatenates JSONL chunks from stdin.
func cmdReassembleTranscript() error {
	var input struct {
		Chunks [][]byte `json:"chunks"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return fmt.Errorf("failed to parse stdin: %w", err)
	}

	result := agent.ReassembleJSONL(input.Chunks)
	if _, err := os.Stdout.Write(result); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}

// cmdGetTranscriptPosition returns the line count of the transcript file.
func cmdGetTranscriptPosition(path string) error {
	if path == "" {
		return writeJSON(map[string]int{"position": 0})
	}

	file, err := os.Open(path) //nolint:gosec // Path from protocol input
	if err != nil {
		if os.IsNotExist(err) {
			return writeJSON(map[string]int{"position": 0})
		}
		return fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	lineCount := 0

	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			if readErr == io.EOF {
				if len(line) > 0 {
					lineCount++
				}
				break
			}
			return fmt.Errorf("failed to read transcript: %w", readErr)
		}
		lineCount++
	}

	return writeJSON(map[string]int{"position": lineCount})
}

// cmdExtractModifiedFiles always returns empty since Cursor transcripts
// do not contain tool_use blocks for file detection.
func cmdExtractModifiedFiles() error {
	return writeJSON(struct {
		Files           []string `json:"files"`
		CurrentPosition int      `json:"current_position"`
	}{
		Files:           nil,
		CurrentPosition: 0,
	})
}

// cmdExtractPrompts extracts user prompts from the transcript.
func cmdExtractPrompts(sessionRef string, fromOffset int) error {
	lines, err := transcript.ParseFromFileAtLine(sessionRef, fromOffset)
	if err != nil {
		return fmt.Errorf("failed to parse transcript: %w", err)
	}

	var prompts []string
	for i := range lines {
		if lines[i].Type != transcript.TypeUser {
			continue
		}
		content := transcript.ExtractUserContent(lines[i].Message)
		if content != "" {
			prompts = append(prompts, textutil.StripIDEContextTags(content))
		}
	}

	return writeJSON(struct {
		Prompts []string `json:"prompts"`
	}{Prompts: prompts})
}

// cmdExtractSummary extracts the last assistant text block as a summary.
func cmdExtractSummary(sessionRef string) error {
	data, err := os.ReadFile(sessionRef) //nolint:gosec // Path from protocol input
	if err != nil {
		return fmt.Errorf("failed to read transcript: %w", err)
	}

	lines, parseErr := transcript.ParseFromBytes(data)
	if parseErr != nil {
		return fmt.Errorf("failed to parse transcript: %w", parseErr)
	}

	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Type != transcript.TypeAssistant {
			continue
		}
		var msg transcript.AssistantMessage
		if err := json.Unmarshal(lines[i].Message, &msg); err != nil {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == transcript.ContentTypeText && block.Text != "" {
				return writeJSON(struct {
					Summary    string `json:"summary"`
					HasSummary bool   `json:"has_summary"`
				}{Summary: block.Text, HasSummary: true})
			}
		}
	}

	return writeJSON(struct {
		Summary    string `json:"summary"`
		HasSummary bool   `json:"has_summary"`
	}{Summary: "", HasSummary: false})
}
