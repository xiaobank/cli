package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
)

func cmdGetTranscriptPosition() error {
	fs := flag.NewFlagSet("get-transcript-position", flag.ContinueOnError)
	path := fs.String("path", "", "transcript file path")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	if *path == "" {
		return writeJSON(map[string]any{"position": 0})
	}

	file, err := os.Open(*path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeJSON(map[string]any{"position": 0})
		}
		return fmt.Errorf("failed to open transcript file: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	lineCount := 0

	for {
		lineData, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			if readErr == io.EOF {
				if len(lineData) > 0 {
					lineCount++
				}
				break
			}
			return fmt.Errorf("failed to read transcript: %w", readErr)
		}
		lineCount++
	}

	return writeJSON(map[string]any{"position": lineCount})
}

func cmdExtractModifiedFiles() error {
	// Cursor transcripts lack tool_use blocks, so always return nil.
	return writeJSON(map[string]any{
		"files":            nil,
		"current_position": 0,
	})
}

func cmdExtractPrompts() error {
	fs := flag.NewFlagSet("extract-prompts", flag.ContinueOnError)
	sessionRef := fs.String("session-ref", "", "session reference path")
	offset := fs.String("offset", "0", "line offset")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	fromOffset, err := strconv.Atoi(*offset)
	if err != nil {
		fromOffset = 0
	}

	lines, err := parseFromFileAtLine(*sessionRef, fromOffset)
	if err != nil {
		return fmt.Errorf("failed to parse transcript: %w", err)
	}

	var prompts []string
	for i := range lines {
		if lines[i].Type != typeUser {
			continue
		}
		content := extractUserContent(lines[i].Message)
		if content != "" {
			prompts = append(prompts, stripIDEContextTags(content))
		}
	}

	if prompts == nil {
		prompts = []string{}
	}
	return writeJSON(map[string]any{"prompts": prompts})
}

func cmdExtractSummary() error {
	fs := flag.NewFlagSet("extract-summary", flag.ContinueOnError)
	sessionRef := fs.String("session-ref", "", "session reference path")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	data, err := os.ReadFile(*sessionRef)
	if err != nil {
		return fmt.Errorf("failed to read transcript: %w", err)
	}

	lines, parseErr := parseFromBytes(data)
	if parseErr != nil {
		return fmt.Errorf("failed to parse transcript: %w", parseErr)
	}

	// Walk backward to find last assistant text block
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Type != typeAssistant {
			continue
		}
		var msg assistantMessage
		if err := json.Unmarshal(lines[i].Message, &msg); err != nil {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == contentTypeText && block.Text != "" {
				return writeJSON(map[string]any{
					"summary":     block.Text,
					"has_summary": true,
				})
			}
		}
	}

	return writeJSON(map[string]any{
		"summary":     "",
		"has_summary": false,
	})
}
