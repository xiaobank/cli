package kiro

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- InstallHooks ---

func TestInstallHooks_FreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}
	count, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}
	if count != 5 {
		t.Errorf("InstallHooks() returned count %d, want 5", count)
	}

	file := readKiroAgentFile(t, tempDir)

	if file.Name != "entire" {
		t.Errorf("Name = %q, want %q", file.Name, "entire")
	}
	if len(file.Tools) == 0 {
		t.Error("Tools should not be empty")
	}
	assertKiroHookCommand(t, file.Hooks.AgentSpawn, "entire hooks kiro agent-spawn")
	assertKiroHookCommand(t, file.Hooks.UserPromptSubmit, "entire hooks kiro user-prompt-submit")
	assertKiroHookCommand(t, file.Hooks.PreToolUse, "entire hooks kiro pre-tool-use")
	assertKiroHookCommand(t, file.Hooks.PostToolUse, "entire hooks kiro post-tool-use")
	assertKiroHookCommand(t, file.Hooks.Stop, "entire hooks kiro stop")
}

func TestInstallHooks_LocalDevMode(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}
	count, err := ag.InstallHooks(context.Background(), true, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}
	if count != 5 {
		t.Errorf("InstallHooks() returned count %d, want 5", count)
	}

	file := readKiroAgentFile(t, tempDir)

	expectedPrefix := "go run ${KIRO_PROJECT_DIR}/cmd/entire/main.go hooks kiro "
	assertKiroHookCommand(t, file.Hooks.AgentSpawn, expectedPrefix+"agent-spawn")
	assertKiroHookCommand(t, file.Hooks.Stop, expectedPrefix+"stop")
}

func TestInstallHooks_Idempotent(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}

	// First install
	count1, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("first InstallHooks() error = %v", err)
	}
	if count1 != 5 {
		t.Errorf("first InstallHooks() count = %d, want 5", count1)
	}

	// Second install should detect hooks are already current and skip
	count2, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("second InstallHooks() error = %v", err)
	}
	if count2 != 0 {
		t.Errorf("second InstallHooks() count = %d, want 0 (no new hooks)", count2)
	}
}

func TestInstallHooks_ForceReinstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}

	// First install
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("first InstallHooks() error = %v", err)
	}

	// Force reinstall should overwrite
	count, err := ag.InstallHooks(context.Background(), false, true)
	if err != nil {
		t.Fatalf("force InstallHooks() error = %v", err)
	}
	if count != 5 {
		t.Errorf("force InstallHooks() count = %d, want 5", count)
	}
}

func TestInstallHooks_IncludesAllDefaultTools(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	file := readKiroAgentFile(t, tempDir)

	expectedTools := []string{
		"read", "write", "shell", "grep", "glob",
		"aws", "report", "introspect", "knowledge",
		"thinking", "todo", "delegate",
	}

	if len(file.Tools) != len(expectedTools) {
		t.Fatalf("Tools count = %d, want %d", len(file.Tools), len(expectedTools))
	}

	for i, want := range expectedTools {
		if file.Tools[i] != want {
			t.Errorf("Tools[%d] = %q, want %q", i, file.Tools[i], want)
		}
	}
}

func TestInstallHooks_CreatesDirectoryStructure(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	hooksPath := filepath.Join(tempDir, ".kiro", "agents", "entire.json")
	if _, err := os.Stat(hooksPath); err != nil {
		t.Errorf("hooks file not found at %s: %v", hooksPath, err)
	}
}

func TestInstallHooks_FilePermissions(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	hooksPath := filepath.Join(tempDir, ".kiro", "agents", "entire.json")
	info, err := os.Stat(hooksPath)
	if err != nil {
		t.Fatalf("failed to stat hooks file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("hooks file permissions = %o, want %o", perm, 0o600)
	}
}

func TestInstallHooks_ProducesValidJSON(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	hooksPath := filepath.Join(tempDir, ".kiro", "agents", "entire.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks file: %v", err)
	}

	if !json.Valid(data) {
		t.Error("hooks file content is not valid JSON")
	}

	// Verify trailing newline (jsonutil.MarshalIndentWithNewline)
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Error("hooks file should end with a newline")
	}
}

func TestInstallHooks_SwitchFromLocalDevToProduction(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}

	// Install in localDev mode
	_, err := ag.InstallHooks(context.Background(), true, false)
	if err != nil {
		t.Fatalf("localDev InstallHooks() error = %v", err)
	}

	// Force-install in production mode
	_, err = ag.InstallHooks(context.Background(), false, true)
	if err != nil {
		t.Fatalf("production InstallHooks() error = %v", err)
	}

	file := readKiroAgentFile(t, tempDir)
	assertKiroHookCommand(t, file.Hooks.AgentSpawn, "entire hooks kiro agent-spawn")
	assertKiroHookCommand(t, file.Hooks.Stop, "entire hooks kiro stop")
}

// --- UninstallHooks ---

func TestUninstallHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}

	// Install first
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	// Verify file exists
	hooksPath := filepath.Join(tempDir, ".kiro", "agents", "entire.json")
	if _, err := os.Stat(hooksPath); err != nil {
		t.Fatalf("hooks file should exist before uninstall: %v", err)
	}

	// Uninstall
	err = ag.UninstallHooks(context.Background())
	if err != nil {
		t.Fatalf("UninstallHooks() error = %v", err)
	}

	// Verify file is removed
	if _, err := os.Stat(hooksPath); err == nil {
		t.Error("hooks file should be removed after uninstall")
	}
}

func TestUninstallHooks_NoFileExists(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}

	// Should not error when file doesn't exist
	err := ag.UninstallHooks(context.Background())
	if err != nil {
		t.Fatalf("UninstallHooks() should not error when no file exists: %v", err)
	}
}

// --- AreHooksInstalled ---

func TestAreHooksInstalled_AfterInstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}

	// Not installed initially
	if ag.AreHooksInstalled(context.Background()) {
		t.Error("hooks should not be installed initially")
	}

	// Install
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	// Should be installed now
	if !ag.AreHooksInstalled(context.Background()) {
		t.Error("hooks should be installed after InstallHooks()")
	}
}

func TestAreHooksInstalled_AfterUninstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}

	// Install
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}
	if !ag.AreHooksInstalled(context.Background()) {
		t.Fatal("hooks should be installed")
	}

	// Uninstall
	if err := ag.UninstallHooks(context.Background()); err != nil {
		t.Fatalf("UninstallHooks() error = %v", err)
	}

	// Should not be installed after uninstall
	if ag.AreHooksInstalled(context.Background()) {
		t.Error("hooks should not be installed after UninstallHooks()")
	}
}

func TestAreHooksInstalled_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Write invalid JSON to the hooks file
	hooksDir := filepath.Join(tempDir, ".kiro", "agents")
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "entire.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("failed to write invalid JSON: %v", err)
	}

	ag := &KiroAgent{}
	if ag.AreHooksInstalled(context.Background()) {
		t.Error("AreHooksInstalled() should return false for invalid JSON")
	}
}

func TestAreHooksInstalled_EmptyHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Write valid JSON but with empty hooks
	hooksDir := filepath.Join(tempDir, ".kiro", "agents")
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}
	emptyFile := `{"name": "entire", "tools": [], "hooks": {}}`
	if err := os.WriteFile(filepath.Join(hooksDir, "entire.json"), []byte(emptyFile), 0o600); err != nil {
		t.Fatalf("failed to write empty hooks file: %v", err)
	}

	ag := &KiroAgent{}
	if ag.AreHooksInstalled(context.Background()) {
		t.Error("AreHooksInstalled() should return false when hooks section is empty")
	}
}

func TestAreHooksInstalled_OnlyEntireHooksDetected(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Write a file with non-Entire hooks only
	hooksDir := filepath.Join(tempDir, ".kiro", "agents")
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}
	otherHooksFile := `{
		"name": "other-agent",
		"tools": [],
		"hooks": {
			"agentSpawn": [{"command": "some-other-tool agent-spawn"}],
			"stop": [{"command": "some-other-tool stop"}]
		}
	}`
	if err := os.WriteFile(filepath.Join(hooksDir, "entire.json"), []byte(otherHooksFile), 0o600); err != nil {
		t.Fatalf("failed to write other hooks file: %v", err)
	}

	ag := &KiroAgent{}
	if ag.AreHooksInstalled(context.Background()) {
		t.Error("AreHooksInstalled() should return false for non-Entire hooks")
	}
}

func TestAreHooksInstalled_DetectsLocalDevHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &KiroAgent{}

	// Install in localDev mode
	_, err := ag.InstallHooks(context.Background(), true, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	// Should detect localDev hooks as installed
	if !ag.AreHooksInstalled(context.Background()) {
		t.Error("AreHooksInstalled() should detect localDev hooks")
	}
}

// --- Helper functions ---

func readKiroAgentFile(t *testing.T, tempDir string) kiroAgentFile {
	t.Helper()
	hooksPath := filepath.Join(tempDir, ".kiro", "agents", "entire.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read hooks file: %v", err)
	}

	var file kiroAgentFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("failed to parse hooks file: %v", err)
	}
	return file
}

func assertKiroHookCommand(t *testing.T, entries []kiroHookEntry, expectedCommand string) {
	t.Helper()
	if len(entries) == 0 {
		t.Errorf("expected hook entry with command %q, got empty slice", expectedCommand)
		return
	}
	found := false
	for _, entry := range entries {
		if entry.Command == expectedCommand {
			found = true
			break
		}
	}
	if !found {
		commands := make([]string, 0, len(entries))
		for _, entry := range entries {
			commands = append(commands, entry.Command)
		}
		t.Errorf("expected hook command %q, got %v", expectedCommand, commands)
	}
}

// --- Internal helper functions ---

func TestIsEntireHook(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		command string
		want    bool
	}{
		{"entire hooks kiro agent-spawn", true},
		{"entire hooks kiro stop", true},
		{"entire hooks kiro user-prompt-submit", true},
		{localDevCmdPrefix + "hooks kiro stop", true},
		{"some-other-tool agent-spawn", false},
		{"entire-something-else", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.command, func(t *testing.T) {
			t.Parallel()
			got := isEntireHook(tc.command)
			if got != tc.want {
				t.Errorf("isEntireHook(%q) = %v, want %v", tc.command, got, tc.want)
			}
		})
	}
}

func TestHasEntireHook(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		entries []kiroHookEntry
		want    bool
	}{
		{
			name:    "empty",
			entries: nil,
			want:    false,
		},
		{
			name:    "non-entire hook",
			entries: []kiroHookEntry{{Command: "other-tool stop"}},
			want:    false,
		},
		{
			name:    "entire hook",
			entries: []kiroHookEntry{{Command: "entire hooks kiro stop"}},
			want:    true,
		},
		{
			name: "mixed hooks",
			entries: []kiroHookEntry{
				{Command: "other-tool stop"},
				{Command: "entire hooks kiro stop"},
			},
			want: true,
		},
		{
			name:    "local dev hook",
			entries: []kiroHookEntry{{Command: localDevCmdPrefix + "hooks kiro stop"}},
			want:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasEntireHook(tc.entries)
			if got != tc.want {
				t.Errorf("hasEntireHook() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAllHooksPresent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		hooks    kiroHooks
		localDev bool
		want     bool
	}{
		{
			name: "all production hooks present",
			hooks: kiroHooks{
				AgentSpawn:       []kiroHookEntry{{Command: "entire hooks kiro agent-spawn"}},
				UserPromptSubmit: []kiroHookEntry{{Command: "entire hooks kiro user-prompt-submit"}},
				PreToolUse:       []kiroHookEntry{{Command: "entire hooks kiro pre-tool-use"}},
				PostToolUse:      []kiroHookEntry{{Command: "entire hooks kiro post-tool-use"}},
				Stop:             []kiroHookEntry{{Command: "entire hooks kiro stop"}},
			},
			localDev: false,
			want:     true,
		},
		{
			name: "all local dev hooks present",
			hooks: kiroHooks{
				AgentSpawn:       []kiroHookEntry{{Command: localDevCmdPrefix + "hooks kiro agent-spawn"}},
				UserPromptSubmit: []kiroHookEntry{{Command: localDevCmdPrefix + "hooks kiro user-prompt-submit"}},
				PreToolUse:       []kiroHookEntry{{Command: localDevCmdPrefix + "hooks kiro pre-tool-use"}},
				PostToolUse:      []kiroHookEntry{{Command: localDevCmdPrefix + "hooks kiro post-tool-use"}},
				Stop:             []kiroHookEntry{{Command: localDevCmdPrefix + "hooks kiro stop"}},
			},
			localDev: true,
			want:     true,
		},
		{
			name:     "empty hooks",
			hooks:    kiroHooks{},
			localDev: false,
			want:     false,
		},
		{
			name: "missing stop hook",
			hooks: kiroHooks{
				AgentSpawn:       []kiroHookEntry{{Command: "entire hooks kiro agent-spawn"}},
				UserPromptSubmit: []kiroHookEntry{{Command: "entire hooks kiro user-prompt-submit"}},
				PreToolUse:       []kiroHookEntry{{Command: "entire hooks kiro pre-tool-use"}},
				PostToolUse:      []kiroHookEntry{{Command: "entire hooks kiro post-tool-use"}},
			},
			localDev: false,
			want:     false,
		},
		{
			name: "wrong mode - production hooks with localDev=true",
			hooks: kiroHooks{
				AgentSpawn:       []kiroHookEntry{{Command: "entire hooks kiro agent-spawn"}},
				UserPromptSubmit: []kiroHookEntry{{Command: "entire hooks kiro user-prompt-submit"}},
				PreToolUse:       []kiroHookEntry{{Command: "entire hooks kiro pre-tool-use"}},
				PostToolUse:      []kiroHookEntry{{Command: "entire hooks kiro post-tool-use"}},
				Stop:             []kiroHookEntry{{Command: "entire hooks kiro stop"}},
			},
			localDev: true,
			want:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := allHooksPresent(tc.hooks, tc.localDev)
			if got != tc.want {
				t.Errorf("allHooksPresent() = %v, want %v", got, tc.want)
			}
		})
	}
}
