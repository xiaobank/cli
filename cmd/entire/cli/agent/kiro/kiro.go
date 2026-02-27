// Package kiro implements the Agent interface for Kiro (Amazon's AI coding CLI).
package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/agent/types"
	"github.com/entireio/cli/cmd/entire/cli/logging"
	"github.com/entireio/cli/cmd/entire/cli/paths"
)

//nolint:gochecknoinits // Agent self-registration is the intended pattern
func init() {
	agent.Register(agent.AgentNameKiro, NewKiroAgent)
}

//nolint:revive // KiroAgent is clearer than Agent in this context
type KiroAgent struct{}

// NewKiroAgent creates a new Kiro agent instance.
func NewKiroAgent() agent.Agent {
	return &KiroAgent{}
}

// --- Identity ---

func (k *KiroAgent) Name() types.AgentName   { return agent.AgentNameKiro }
func (k *KiroAgent) Type() types.AgentType   { return agent.AgentTypeKiro }
func (k *KiroAgent) Description() string     { return "Kiro - Amazon's AI coding CLI" }
func (k *KiroAgent) IsPreview() bool         { return true }
func (k *KiroAgent) ProtectedDirs() []string { return []string{".kiro"} }

func (k *KiroAgent) DetectPresence(ctx context.Context) (bool, error) {
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		repoRoot = "."
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".kiro")); err == nil {
		return true, nil
	}
	return false, nil
}

// --- Transcript Storage ---

// ReadTranscript reads the transcript for a session.
// The sessionRef is expected to be a path to the cached conversation JSON file.
func (k *KiroAgent) ReadTranscript(sessionRef string) ([]byte, error) {
	data, err := os.ReadFile(sessionRef) //nolint:gosec // Path from agent hook
	if err != nil {
		return nil, fmt.Errorf("failed to read kiro transcript: %w", err)
	}
	return data, nil
}

// ChunkTranscript splits a Kiro conversation JSON transcript by distributing history entries across chunks.
func (k *KiroAgent) ChunkTranscript(_ context.Context, content []byte, maxSize int) ([][]byte, error) {
	var conv Conversation
	if err := json.Unmarshal(content, &conv); err != nil {
		return nil, fmt.Errorf("failed to parse conversation for chunking: %w", err)
	}

	if len(conv.History) == 0 {
		return [][]byte{content}, nil
	}

	// Calculate base size (conversation with empty history)
	baseConv := Conversation{ConversationID: conv.ConversationID}
	baseBytes, err := json.Marshal(baseConv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal base conversation for chunking: %w", err)
	}
	baseSize := len(baseBytes)

	var chunks [][]byte
	var currentEntries []HistoryEntry
	currentSize := baseSize

	for _, entry := range conv.History {
		entryBytes, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal history entry for chunking: %w", err)
		}
		entrySize := len(entryBytes) + 1 // +1 for comma separator

		if currentSize+entrySize > maxSize && len(currentEntries) > 0 {
			chunkData, err := json.Marshal(Conversation{
				ConversationID: conv.ConversationID,
				History:        currentEntries,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to marshal chunk: %w", err)
			}
			chunks = append(chunks, chunkData)

			currentEntries = nil
			currentSize = baseSize
		}

		currentEntries = append(currentEntries, entry)
		currentSize += entrySize
	}

	if len(currentEntries) > 0 {
		chunkData, err := json.Marshal(Conversation{
			ConversationID: conv.ConversationID,
			History:        currentEntries,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal final chunk: %w", err)
		}
		chunks = append(chunks, chunkData)
	}

	if len(chunks) == 0 {
		return nil, errors.New("failed to create any chunks")
	}

	return chunks, nil
}

// ReassembleTranscript merges Kiro conversation JSON chunks by combining their history arrays.
func (k *KiroAgent) ReassembleTranscript(chunks [][]byte) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, errors.New("no chunks to reassemble")
	}

	var allEntries []HistoryEntry
	var convID string

	for i, chunk := range chunks {
		var conv Conversation
		if err := json.Unmarshal(chunk, &conv); err != nil {
			return nil, fmt.Errorf("failed to unmarshal chunk %d: %w", i, err)
		}
		if i == 0 {
			convID = conv.ConversationID
		}
		allEntries = append(allEntries, conv.History...)
	}

	result, err := json.Marshal(Conversation{
		ConversationID: convID,
		History:        allEntries,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reassembled transcript: %w", err)
	}
	return result, nil
}

// --- Legacy methods ---

func (k *KiroAgent) GetSessionID(input *agent.HookInput) string {
	return input.SessionID
}

// GetSessionDir returns the directory where Entire stores Kiro session transcripts.
// Stored in os.TempDir()/entire-kiro/<sanitized-path>/ to avoid squatting on
// Kiro's own directories (.kiro/ is project-level).
func (k *KiroAgent) GetSessionDir(repoPath string) (string, error) {
	if override := os.Getenv("ENTIRE_TEST_KIRO_PROJECT_DIR"); override != "" {
		return override, nil
	}

	projectDir := SanitizePathForKiro(repoPath)
	return filepath.Join(os.TempDir(), "entire-kiro", projectDir), nil
}

func (k *KiroAgent) ResolveSessionFile(sessionDir, agentSessionID string) string {
	return filepath.Join(sessionDir, agentSessionID+".json")
}

func (k *KiroAgent) ReadSession(input *agent.HookInput) (*agent.AgentSession, error) {
	if input.SessionRef == "" {
		return nil, errors.New("no session ref provided")
	}
	data, err := os.ReadFile(input.SessionRef)
	if err != nil {
		return nil, fmt.Errorf("failed to read session: %w", err)
	}

	modifiedFiles, err := ExtractModifiedFiles(data)
	if err != nil {
		logging.Warn(context.Background(), "failed to extract modified files from kiro session",
			slog.String("session_ref", input.SessionRef),
			slog.String("error", err.Error()),
		)
		modifiedFiles = nil
	}

	return &agent.AgentSession{
		AgentName:     k.Name(),
		SessionID:     input.SessionID,
		SessionRef:    input.SessionRef,
		NativeData:    data,
		ModifiedFiles: modifiedFiles,
	}, nil
}

func (k *KiroAgent) WriteSession(_ context.Context, session *agent.AgentSession) error {
	if session == nil {
		return errors.New("nil session")
	}
	if len(session.NativeData) == 0 {
		return errors.New("no session data to write")
	}

	// Kiro uses SQLite — we cannot easily write back without the kiro-cli.
	// For now, write to the cached transcript file so rewind can restore it.
	if session.SessionRef == "" {
		return errors.New("no session ref for write")
	}

	dir := filepath.Dir(session.SessionRef)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	if err := os.WriteFile(session.SessionRef, session.NativeData, 0o600); err != nil {
		return fmt.Errorf("failed to write session data: %w", err)
	}

	return nil
}

func (k *KiroAgent) FormatResumeCommand(_ string) string {
	return "kiro-cli"
}

// nonAlphanumericRegex matches any non-alphanumeric character.
var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]`)

// SanitizePathForKiro converts a path to a safe directory name.
func SanitizePathForKiro(path string) string {
	return nonAlphanumericRegex.ReplaceAllString(path, "-")
}

// ExtractModifiedFiles extracts modified file paths from raw conversation JSON bytes.
func ExtractModifiedFiles(data []byte) ([]string, error) {
	conv, err := ParseConversation(data)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var files []string

	for _, entry := range conv.History {
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

	return files, nil
}

// ParseConversation parses raw JSON content into a Conversation structure.
func ParseConversation(data []byte) (*Conversation, error) {
	if len(data) == 0 {
		return nil, nil //nolint:nilnil // nil for empty data is expected
	}

	var conv Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, fmt.Errorf("failed to parse kiro conversation: %w", err)
	}

	return &conv, nil
}

func isFileModificationTool(toolName string) bool {
	for _, t := range FileModificationTools {
		if t == toolName {
			return true
		}
	}
	return false
}

// extractFilePathsFromInput extracts file paths from a tool's input.
// The input is typically a map with keys like "file_path", "path", or "filePath".
func extractFilePathsFromInput(input any) []string {
	m, ok := input.(map[string]any)
	if !ok {
		return nil
	}

	for _, key := range []string{"file_path", "path", "filePath"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return []string{s}
			}
		}
	}
	return nil
}
