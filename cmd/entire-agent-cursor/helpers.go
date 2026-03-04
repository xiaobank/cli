// Binary entire-agent-cursor implements the external agent protocol for Cursor.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// writeJSON marshals v as JSON and writes it to stdout.
func writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}

// fatal prints msg to stderr and exits 1.
func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

// repoRoot returns the repository root. It first checks the ENTIRE_REPO_ROOT
// env var (set by the CLI), then falls back to git rev-parse.
func repoRoot() (string, error) {
	if root := os.Getenv("ENTIRE_REPO_ROOT"); root != "" {
		return root, nil
	}
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// marshalIndentWithNewline is like json.MarshalIndent but adds a trailing newline.
func marshalIndentWithNewline(v any, prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent(prefix, indent)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encoding JSON: %w", err)
	}
	return buf.Bytes(), nil
}

// chunkJSONL splits JSONL content at line boundaries.
func chunkJSONL(content []byte, maxSize int) ([][]byte, error) {
	if len(content) == 0 {
		return [][]byte{}, nil
	}

	lines := strings.Split(string(content), "\n")
	var chunks [][]byte
	var currentChunk strings.Builder

	for i, line := range lines {
		lineWithNewline := line + "\n"

		if len(lineWithNewline) > maxSize {
			return nil, fmt.Errorf("JSONL line %d exceeds maximum chunk size (%d bytes > %d bytes)", i+1, len(lineWithNewline), maxSize)
		}

		if currentChunk.Len()+len(lineWithNewline) > maxSize && currentChunk.Len() > 0 {
			chunks = append(chunks, []byte(strings.TrimSuffix(currentChunk.String(), "\n")))
			currentChunk.Reset()
		}
		currentChunk.WriteString(lineWithNewline)
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, []byte(strings.TrimSuffix(currentChunk.String(), "\n")))
	}

	return chunks, nil
}

// reassembleJSONL concatenates JSONL chunks with newlines.
func reassembleJSONL(chunks [][]byte) []byte {
	var result strings.Builder
	for i, chunk := range chunks {
		result.Write(chunk)
		if i < len(chunks)-1 {
			result.WriteString("\n")
		}
	}
	return []byte(result.String())
}

// writeNull writes the JSON literal "null" to stdout.
func writeNull() error {
	if _, err := os.Stdout.Write([]byte("null")); err != nil {
		return fmt.Errorf("failed to write null: %w", err)
	}
	return nil
}

// readJSON reads all stdin and unmarshals JSON into the given type.
func readJSON[T any]() (*T, error) {
	data, err := readStdin()
	if err != nil {
		return nil, err
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

// readStdin reads all bytes from stdin.
func readStdin() ([]byte, error) {
	data, err := os.ReadFile("/dev/stdin")
	if err != nil {
		return nil, fmt.Errorf("failed to read stdin: %w", err)
	}
	return data, nil
}

// stripIDEContextTags removes IDE-injected context tags from prompt text.
func stripIDEContextTags(text string) string {
	result := ideContextTagRegex.ReplaceAllString(text, "")
	for _, re := range systemTagRegexes {
		result = re.ReplaceAllString(result, "")
	}
	return strings.TrimSpace(result)
}
