package kiro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/paths"
)

// Hook name constants — these become CLI subcommands under `entire hooks kiro`.
const (
	HookNameAgentSpawn       = "agent-spawn"
	HookNameUserPromptSubmit = "user-prompt-submit"
	HookNamePreToolUse       = "pre-tool-use"
	HookNamePostToolUse      = "post-tool-use"
	HookNameStop             = "stop"
)

// HookNames returns the hook verbs this agent supports.
func (k *KiroAgent) HookNames() []string {
	return []string{
		HookNameAgentSpawn,
		HookNameUserPromptSubmit,
		HookNamePreToolUse,
		HookNamePostToolUse,
		HookNameStop,
	}
}

// ParseHookEvent translates Kiro hook calls into normalized lifecycle events.
func (k *KiroAgent) ParseHookEvent(ctx context.Context, hookName string, stdin io.Reader) (*agent.Event, error) {
	switch hookName {
	case HookNameAgentSpawn:
		raw, err := agent.ReadAndParseHookInput[hookInputRaw](stdin)
		if err != nil {
			return nil, err
		}

		sessionID, err := k.querySessionID(ctx, raw.CWD)
		if err != nil {
			return nil, fmt.Errorf("querying kiro session ID: %w", err)
		}

		return &agent.Event{
			Type:      agent.SessionStart,
			SessionID: sessionID,
			Timestamp: time.Now(),
		}, nil

	case HookNameUserPromptSubmit:
		raw, err := agent.ReadAndParseHookInput[hookInputRaw](stdin)
		if err != nil {
			return nil, err
		}

		sessionID, err := k.querySessionID(ctx, raw.CWD)
		if err != nil {
			return nil, fmt.Errorf("querying kiro session ID: %w", err)
		}

		repoRoot, err := paths.WorktreeRoot(ctx)
		if err != nil {
			repoRoot = "."
		}
		tmpDir := filepath.Join(repoRoot, paths.EntireTmpDir)
		transcriptPath := filepath.Join(tmpDir, sessionID+".json")

		return &agent.Event{
			Type:       agent.TurnStart,
			SessionID:  sessionID,
			SessionRef: transcriptPath,
			Prompt:     raw.Prompt,
			Timestamp:  time.Now(),
		}, nil

	case HookNamePreToolUse, HookNamePostToolUse:
		// Pass-through hooks with no lifecycle significance
		return nil, nil //nolint:nilnil // nil event = no lifecycle action for pass-through hooks

	case HookNameStop:
		raw, err := agent.ReadAndParseHookInput[hookInputRaw](stdin)
		if err != nil {
			return nil, err
		}

		sessionID, err := k.querySessionID(ctx, raw.CWD)
		if err != nil {
			return nil, fmt.Errorf("querying kiro session ID: %w", err)
		}

		transcriptPath, exportErr := k.fetchAndCacheTranscript(ctx, sessionID, raw.CWD)
		if exportErr != nil {
			return nil, fmt.Errorf("failed to cache kiro transcript: %w", exportErr)
		}

		return &agent.Event{
			Type:       agent.TurnEnd,
			SessionID:  sessionID,
			SessionRef: transcriptPath,
			Timestamp:  time.Now(),
		}, nil

	default:
		return nil, nil //nolint:nilnil // nil event = no lifecycle action for unknown hooks
	}
}

// PrepareTranscript ensures the Kiro transcript file is up-to-date by querying SQLite.
func (k *KiroAgent) PrepareTranscript(ctx context.Context, sessionRef string) error {
	if _, err := os.Stat(sessionRef); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat Kiro transcript path %s: %w", sessionRef, err)
	}

	base := filepath.Base(sessionRef)
	if !strings.HasSuffix(base, ".json") {
		return fmt.Errorf("invalid Kiro transcript path (expected .json): %s", sessionRef)
	}
	sessionID := strings.TrimSuffix(base, ".json")
	if sessionID == "" {
		return fmt.Errorf("empty session ID in transcript path: %s", sessionRef)
	}

	// Use CWD as the project directory for the SQLite query
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		repoRoot = "."
	}

	_, err = k.fetchAndCacheTranscript(ctx, sessionID, repoRoot)
	return err
}

// querySessionID queries the Kiro SQLite database to find the most recent
// conversation_id for the given working directory.
func (k *KiroAgent) querySessionID(ctx context.Context, cwd string) (string, error) {
	// Mock mode for integration tests
	if os.Getenv("ENTIRE_TEST_KIRO_MOCK_DB") != "" {
		// In mock mode, use the CWD-based session ID from the test fixture
		return mockSessionID(cwd), nil
	}

	dbPath, err := kiroDBPath()
	if err != nil {
		return "", err
	}

	query := fmt.Sprintf(
		`SELECT json_extract(value, '$.conversation_id') FROM conversations_v2 WHERE key = '%s' ORDER BY updated_at DESC LIMIT 1`,
		escapeSQLString(cwd),
	)

	out, err := runSQLite3(ctx, dbPath, query)
	if err != nil {
		return "", fmt.Errorf("sqlite3 query failed: %w", err)
	}

	sessionID := strings.TrimSpace(out)
	if sessionID == "" {
		return "", fmt.Errorf("no kiro conversation found for cwd: %s", cwd)
	}

	return sessionID, nil
}

// fetchAndCacheTranscript queries Kiro's SQLite database for the conversation
// value and writes it to a temporary JSON file.
//
// Integration testing: Set ENTIRE_TEST_KIRO_MOCK_DB=1 to skip the SQLite
// query and use pre-written mock data instead. Tests must pre-write the
// transcript file to .entire/tmp/<sessionID>.json before triggering the hook.
func (k *KiroAgent) fetchAndCacheTranscript(ctx context.Context, sessionID string, cwd string) (string, error) {
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		repoRoot = "."
	}

	tmpDir := filepath.Join(repoRoot, paths.EntireTmpDir)
	tmpFile := filepath.Join(tmpDir, sessionID+".json")

	// Integration test mode: use pre-written mock file
	if os.Getenv("ENTIRE_TEST_KIRO_MOCK_DB") != "" {
		if _, err := os.Stat(tmpFile); err == nil {
			return tmpFile, nil
		}
		return "", fmt.Errorf("mock transcript file not found: %s (ENTIRE_TEST_KIRO_MOCK_DB is set)", tmpFile)
	}

	dbPath, err := kiroDBPath()
	if err != nil {
		return "", err
	}

	// Query the conversation value from SQLite
	query := fmt.Sprintf(
		`SELECT value FROM conversations_v2 WHERE json_extract(value, '$.conversation_id') = '%s' LIMIT 1`,
		escapeSQLString(sessionID),
	)

	data, err := runSQLite3(ctx, dbPath, query)
	if err != nil {
		return "", fmt.Errorf("sqlite3 query failed: %w", err)
	}

	data = strings.TrimSpace(data)
	if data == "" {
		// Fallback: try querying by CWD
		query = fmt.Sprintf(
			`SELECT value FROM conversations_v2 WHERE key = '%s' ORDER BY updated_at DESC LIMIT 1`,
			escapeSQLString(cwd),
		)
		data, err = runSQLite3(ctx, dbPath, query)
		if err != nil {
			return "", fmt.Errorf("sqlite3 fallback query failed: %w", err)
		}
		data = strings.TrimSpace(data)
	}

	if data == "" {
		return "", fmt.Errorf("no kiro conversation found for session: %s", sessionID)
	}

	// Validate output is valid JSON before caching
	if !json.Valid([]byte(data)) {
		return "", fmt.Errorf("kiro sqlite3 returned invalid JSON (%d bytes)", len(data))
	}

	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	if err := os.WriteFile(tmpFile, []byte(data), 0o600); err != nil {
		return "", fmt.Errorf("failed to write transcript file: %w", err)
	}

	return tmpFile, nil
}

// kiroDBPath returns the path to Kiro's SQLite database.
func kiroDBPath() (string, error) {
	if override := os.Getenv("ENTIRE_TEST_KIRO_DB_PATH"); override != "" {
		return override, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "kiro-cli", "data.sqlite3"), nil
	case "linux":
		// XDG_DATA_HOME or fallback to ~/.local/share
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(homeDir, ".local", "share")
		}
		return filepath.Join(dataHome, "kiro-cli", "data.sqlite3"), nil
	default:
		return filepath.Join(homeDir, ".kiro-cli", "data.sqlite3"), nil
	}
}

// runSQLite3 executes a SQL query against the given SQLite database using the sqlite3 CLI.
func runSQLite3(ctx context.Context, dbPath string, query string) (string, error) {
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", dbPath, query)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("sqlite3 command failed: %w", err)
	}

	// sqlite3 -json returns an array of objects
	// For single-column queries, we extract the first column value
	result := strings.TrimSpace(string(out))
	if result == "" || result == "[]" {
		return "", nil
	}

	// Parse the JSON array result
	var rows []map[string]any
	if unmarshalErr := json.Unmarshal([]byte(result), &rows); unmarshalErr != nil {
		// If not valid JSON array, return raw output (simple text mode fallback)
		return result, nil //nolint:nilerr // intentional fallback to raw text
	}

	if len(rows) == 0 {
		return "", nil
	}

	// Return the first column value of the first row
	for _, v := range rows[0] {
		if s, ok := v.(string); ok {
			return s, nil
		}
		b, marshalErr := json.Marshal(v)
		if marshalErr != nil {
			return fmt.Sprintf("%v", v), nil //nolint:nilerr // best-effort string conversion
		}
		return string(b), nil
	}

	return "", nil
}

// escapeSQLString escapes single quotes in SQL strings to prevent injection.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// mockSessionID generates a deterministic session ID from the CWD for testing.
func mockSessionID(cwd string) string {
	// Use a simple hash-like approach for deterministic test session IDs
	sanitized := SanitizePathForKiro(cwd)
	if len(sanitized) > 32 {
		sanitized = sanitized[:32]
	}
	return "mock-session-" + sanitized
}
