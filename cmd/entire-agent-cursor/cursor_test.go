package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary builds the binary for testing and returns the path.
func buildBinary(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "entire-agent-cursor")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", binPath, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}
	return binPath
}

// runBinary runs the binary with args and optional stdin, returning stdout.
func runBinary(t *testing.T, binPath string, stdin []byte, env []string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), binPath, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func TestInfo(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	out, err := runBinary(t, bin, nil, nil, "info")
	if err != nil {
		t.Fatalf("info failed: %v", err)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(out, &info); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if info["name"] != agentName {
		t.Errorf("expected name=%s, got %v", agentName, info["name"])
	}
	if info["type"] != "Cursor" {
		t.Errorf("expected type=Cursor, got %v", info["type"])
	}
	pv, ok := info["protocol_version"].(float64)
	if !ok || int(pv) != 1 {
		t.Errorf("expected protocol_version=1, got %v", info["protocol_version"])
	}
	caps, ok := info["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("expected capabilities to be a map")
	}
	if caps["hooks"] != true {
		t.Errorf("expected hooks=true")
	}
	if caps["transcript_analyzer"] != true {
		t.Errorf("expected transcript_analyzer=true")
	}
}

func TestDetect_NoCursorDir(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	out, err := runBinary(t, bin, nil, []string{"ENTIRE_REPO_ROOT=" + tmpDir}, "detect")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}

	var resp map[string]bool
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["present"] {
		t.Error("expected present=false when .cursor/ doesn't exist")
	}
}

func TestDetect_WithCursorDir(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".cursor"), 0o750); err != nil {
		t.Fatal(err)
	}

	out, err := runBinary(t, bin, nil, []string{"ENTIRE_REPO_ROOT=" + tmpDir}, "detect")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}

	var resp map[string]bool
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !resp["present"] {
		t.Error("expected present=true when .cursor/ exists")
	}
}

func TestSanitizePathForCursor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/nodo/work/project", "Users-nodo-work-project"},
		{"/home/user/my-repo", "home-user-my-repo"},
		{"relative/path", "relative-path"},
		{"/a/b/c", "a-b-c"},
	}
	for _, tt := range tests {
		if got := sanitizePathForCursor(tt.input); got != tt.expected {
			t.Errorf("sanitizePathForCursor(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseHook_SessionStart(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	input := `{"conversation_id":"sess-abc","generation_id":"gen-1","model":"gpt-4","hook_event_name":"session-start","cursor_version":"1.0","workspace_roots":[],"user_email":"","transcript_path":"/tmp/test.jsonl"}`
	out, err := runBinary(t, bin, []byte(input), nil, "parse-hook", "--hook", "session-start")
	if err != nil {
		t.Fatalf("parse-hook failed: %v", err)
	}

	var event eventJSON
	if err := json.Unmarshal(out, &event); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if event.Type != eventSessionStart {
		t.Errorf("expected type=%d, got %d", eventSessionStart, event.Type)
	}
	if event.SessionID != "sess-abc" {
		t.Errorf("expected session_id=sess-abc, got %s", event.SessionID)
	}
	if event.SessionRef != "/tmp/test.jsonl" {
		t.Errorf("expected session_ref=/tmp/test.jsonl, got %s", event.SessionRef)
	}
}

func TestParseHook_BeforeSubmitPrompt(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	input := `{"conversation_id":"sess-abc","generation_id":"gen-1","model":"gpt-4","hook_event_name":"before-submit-prompt","cursor_version":"1.0","workspace_roots":[],"user_email":"","transcript_path":"/tmp/test.jsonl","prompt":"hello world"}`
	out, err := runBinary(t, bin, []byte(input), nil, "parse-hook", "--hook", "before-submit-prompt")
	if err != nil {
		t.Fatalf("parse-hook failed: %v", err)
	}

	var event eventJSON
	if err := json.Unmarshal(out, &event); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if event.Type != eventTurnStart {
		t.Errorf("expected type=%d, got %d", eventTurnStart, event.Type)
	}
	if event.Prompt != "hello world" {
		t.Errorf("expected prompt='hello world', got %q", event.Prompt)
	}
}

func TestParseHook_SubagentStartNoTask(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	input := `{"conversation_id":"sess-abc","generation_id":"gen-1","model":"gpt-4","hook_event_name":"subagent-start","cursor_version":"1.0","workspace_roots":[],"user_email":"","transcript_path":"","subagent_id":"sub-1","subagent_type":"worker","task":""}`
	out, err := runBinary(t, bin, []byte(input), nil, "parse-hook", "--hook", "subagent-start")
	if err != nil {
		t.Fatalf("parse-hook failed: %v", err)
	}

	if strings.TrimSpace(string(out)) != "null" {
		t.Errorf("expected null for subagent-start with no task, got %s", out)
	}
}

func TestParseHook_UnknownHook(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	out, err := runBinary(t, bin, []byte("{}"), nil, "parse-hook", "--hook", "unknown-hook")
	if err != nil {
		t.Fatalf("parse-hook failed: %v", err)
	}

	if strings.TrimSpace(string(out)) != "null" {
		t.Errorf("expected null for unknown hook, got %s", out)
	}
}

func TestResolveSessionFile_NestedLayout(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	sessionID := "test-session"
	nestedDir := filepath.Join(tmpDir, sessionID)
	if err := os.MkdirAll(nestedDir, 0o750); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(nestedDir, sessionID+".jsonl")
	if err := os.WriteFile(nestedFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runBinary(t, bin, nil, nil, "resolve-session-file",
		"--session-dir", tmpDir, "--session-id", sessionID)
	if err != nil {
		t.Fatalf("resolve-session-file failed: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["session_file"] != nestedFile {
		t.Errorf("expected nested path %s, got %s", nestedFile, resp["session_file"])
	}
}

func TestResolveSessionFile_FlatLayout(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	sessionID := "test-session"

	out, err := runBinary(t, bin, nil, nil, "resolve-session-file",
		"--session-dir", tmpDir, "--session-id", sessionID)
	if err != nil {
		t.Fatalf("resolve-session-file failed: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	expected := filepath.Join(tmpDir, sessionID+".jsonl")
	if resp["session_file"] != expected {
		t.Errorf("expected flat path %s, got %s", expected, resp["session_file"])
	}
}

func TestGetTranscriptPosition(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")
	content := `{"type":"user","uuid":"1","message":{}}
{"type":"assistant","uuid":"2","message":{}}
{"type":"user","uuid":"3","message":{}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runBinary(t, bin, nil, nil, "get-transcript-position", "--path", path)
	if err != nil {
		t.Fatalf("get-transcript-position failed: %v", err)
	}

	var resp map[string]int
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["position"] != 3 {
		t.Errorf("expected position=3, got %d", resp["position"])
	}
}

func TestGetTranscriptPosition_NonexistentFile(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	out, err := runBinary(t, bin, nil, nil, "get-transcript-position", "--path", "/nonexistent/file.jsonl")
	if err != nil {
		t.Fatalf("get-transcript-position failed: %v", err)
	}

	var resp map[string]int
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["position"] != 0 {
		t.Errorf("expected position=0 for nonexistent file, got %d", resp["position"])
	}
}

func TestExtractModifiedFiles_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	out, err := runBinary(t, bin, nil, nil, "extract-modified-files", "--path", "/any", "--offset", "0")
	if err != nil {
		t.Fatalf("extract-modified-files failed: %v", err)
	}

	var resp struct {
		Files           []string `json:"files"`
		CurrentPosition int      `json:"current_position"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Files != nil {
		t.Errorf("expected nil files, got %v", resp.Files)
	}
	if resp.CurrentPosition != 0 {
		t.Errorf("expected current_position=0, got %d", resp.CurrentPosition)
	}
}

func TestFormatResumeCommand(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	out, err := runBinary(t, bin, nil, nil, "format-resume-command", "--session-id", "test")
	if err != nil {
		t.Fatalf("format-resume-command failed: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["command"] == "" {
		t.Error("expected non-empty resume command")
	}
}

func TestInstallAndUninstallHooks(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".cursor"), 0o750); err != nil {
		t.Fatal(err)
	}

	env := []string{"ENTIRE_REPO_ROOT=" + tmpDir}

	// Install hooks
	out, err := runBinary(t, bin, nil, env, "install-hooks")
	if err != nil {
		t.Fatalf("install-hooks failed: %v", err)
	}

	var installResp struct {
		HooksInstalled int `json:"hooks_installed"`
	}
	if err := json.Unmarshal(out, &installResp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if installResp.HooksInstalled != 7 {
		t.Errorf("expected 7 hooks installed, got %d", installResp.HooksInstalled)
	}

	// Verify hooks are installed
	out, err = runBinary(t, bin, nil, env, "are-hooks-installed")
	if err != nil {
		t.Fatalf("are-hooks-installed failed: %v", err)
	}

	var areInstalled map[string]bool
	if err := json.Unmarshal(out, &areInstalled); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !areInstalled["installed"] {
		t.Error("expected installed=true after install")
	}

	// Install again (idempotent) - should install 0
	out, err = runBinary(t, bin, nil, env, "install-hooks")
	if err != nil {
		t.Fatalf("install-hooks second call failed: %v", err)
	}
	if err := json.Unmarshal(out, &installResp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if installResp.HooksInstalled != 0 {
		t.Errorf("expected 0 hooks installed on second call, got %d", installResp.HooksInstalled)
	}

	// Uninstall hooks
	_, err = runBinary(t, bin, nil, env, "uninstall-hooks")
	if err != nil {
		t.Fatalf("uninstall-hooks failed: %v", err)
	}

	// Verify hooks are uninstalled
	out, err = runBinary(t, bin, nil, env, "are-hooks-installed")
	if err != nil {
		t.Fatalf("are-hooks-installed failed: %v", err)
	}
	if err := json.Unmarshal(out, &areInstalled); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if areInstalled["installed"] {
		t.Error("expected installed=false after uninstall")
	}
}

func TestInstallHooks_PreservesExistingHooks(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Write a hooks file with an existing user hook
	existing := `{"version":1,"hooks":{"sessionStart":[{"command":"my-custom-hook start"}]}}`
	if err := os.WriteFile(filepath.Join(cursorDir, "hooks.json"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	env := []string{"ENTIRE_REPO_ROOT=" + tmpDir}
	_, err := runBinary(t, bin, nil, env, "install-hooks")
	if err != nil {
		t.Fatalf("install-hooks failed: %v", err)
	}

	// Read the hooks file and verify user hook is preserved
	data, err := os.ReadFile(filepath.Join(cursorDir, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), "my-custom-hook start") {
		t.Error("existing user hook was not preserved")
	}
	if !strings.Contains(string(data), "entire hooks cursor session-start") {
		t.Error("entire hook was not added")
	}
}

func TestReadAndWriteSession(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "test.jsonl")
	content := `{"type":"user","uuid":"1","message":{"content":"hello"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Read session
	hookInput := hookInputJSON{
		SessionID:  "test-sess",
		SessionRef: transcriptPath,
	}
	inputData, err := json.Marshal(hookInput)
	if err != nil {
		t.Fatalf("failed to marshal hook input: %v", err)
	}

	out, err := runBinary(t, bin, inputData, nil, "read-session")
	if err != nil {
		t.Fatalf("read-session failed: %v", err)
	}

	var session agentSessionJSON
	if err := json.Unmarshal(out, &session); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if session.SessionID != "test-sess" {
		t.Errorf("expected session_id=test-sess, got %s", session.SessionID)
	}
	if session.AgentName != agentName {
		t.Errorf("expected agent_name=%s, got %s", agentName, session.AgentName)
	}
	if len(session.NativeData) == 0 {
		t.Error("expected non-empty native data")
	}

	// Write session to a new path
	newPath := filepath.Join(tmpDir, "written.jsonl")
	session.SessionRef = newPath
	writeData, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("failed to marshal session: %v", err)
	}

	_, err = runBinary(t, bin, writeData, nil, "write-session")
	if err != nil {
		t.Fatalf("write-session failed: %v", err)
	}

	written, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != content {
		t.Errorf("written content mismatch: got %q, want %q", written, content)
	}
}

func TestExtractPrompts(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")
	lines := []string{
		`{"type":"user","uuid":"1","message":{"content":"first prompt"}}`,
		`{"type":"assistant","uuid":"2","message":{"content":[{"type":"text","text":"response"}]}}`,
		`{"type":"user","uuid":"3","message":{"content":"second prompt"}}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runBinary(t, bin, nil, nil, "extract-prompts",
		"--session-ref", path, "--offset", "0")
	if err != nil {
		t.Fatalf("extract-prompts failed: %v", err)
	}

	var resp struct {
		Prompts []string `json:"prompts"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.Prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(resp.Prompts))
	}
	if resp.Prompts[0] != "first prompt" {
		t.Errorf("expected first prompt, got %q", resp.Prompts[0])
	}
	if resp.Prompts[1] != "second prompt" {
		t.Errorf("expected second prompt, got %q", resp.Prompts[1])
	}
}

func TestExtractSummary(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")
	lines := []string{
		`{"type":"user","uuid":"1","message":{"content":"do something"}}`,
		`{"type":"assistant","uuid":"2","message":{"content":[{"type":"text","text":"I did the thing."}]}}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runBinary(t, bin, nil, nil, "extract-summary", "--session-ref", path)
	if err != nil {
		t.Fatalf("extract-summary failed: %v", err)
	}

	var resp struct {
		Summary    string `json:"summary"`
		HasSummary bool   `json:"has_summary"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !resp.HasSummary {
		t.Error("expected has_summary=true")
	}
	if resp.Summary != "I did the thing." {
		t.Errorf("expected summary='I did the thing.', got %q", resp.Summary)
	}
}

func TestExtractSummary_NoAssistant(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")
	content := `{"type":"user","uuid":"1","message":{"content":"hello"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runBinary(t, bin, nil, nil, "extract-summary", "--session-ref", path)
	if err != nil {
		t.Fatalf("extract-summary failed: %v", err)
	}

	var resp struct {
		Summary    string `json:"summary"`
		HasSummary bool   `json:"has_summary"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.HasSummary {
		t.Error("expected has_summary=false for user-only transcript")
	}
}

func TestGetSessionDir(t *testing.T) {
	t.Parallel()
	bin := buildBinary(t)

	tmpDir := t.TempDir()
	out, err := runBinary(t, bin, nil,
		[]string{"ENTIRE_TEST_CURSOR_PROJECT_DIR=" + tmpDir},
		"get-session-dir", "--repo-path", "/some/repo")
	if err != nil {
		t.Fatalf("get-session-dir failed: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["session_dir"] != tmpDir {
		t.Errorf("expected session_dir=%s (from env override), got %s", tmpDir, resp["session_dir"])
	}
}
