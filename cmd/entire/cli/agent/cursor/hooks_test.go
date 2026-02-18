package cursor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallHooks_FreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &CursorAgent{}
	count, err := ag.InstallHooks(false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	if count != 7 {
		t.Errorf("InstallHooks() count = %d, want 7", count)
	}

	hooksFile := readHooksFile(t, tempDir)

	// Verify all hooks are present
	if len(hooksFile.Hooks.SessionStart) != 1 {
		t.Errorf("SessionStart hooks = %d, want 1", len(hooksFile.Hooks.SessionStart))
	}
	if len(hooksFile.Hooks.SessionEnd) != 1 {
		t.Errorf("SessionEnd hooks = %d, want 1", len(hooksFile.Hooks.SessionEnd))
	}
	if len(hooksFile.Hooks.BeforeSubmitPrompt) != 1 {
		t.Errorf("BeforeSubmitPrompt hooks = %d, want 1", len(hooksFile.Hooks.BeforeSubmitPrompt))
	}
	if len(hooksFile.Hooks.Stop) != 1 {
		t.Errorf("Stop hooks = %d, want 1", len(hooksFile.Hooks.Stop))
	}
	// PreToolUse has 1 (Task)
	if len(hooksFile.Hooks.PreToolUse) != 1 {
		t.Errorf("PreToolUse hooks = %d, want 1", len(hooksFile.Hooks.PreToolUse))
	}
	// PostToolUse has 2 (Task + TodoWrite)
	if len(hooksFile.Hooks.PostToolUse) != 2 {
		t.Errorf("PostToolUse hooks = %d, want 2", len(hooksFile.Hooks.PostToolUse))
	}

	// Verify version
	if hooksFile.Version != 1 {
		t.Errorf("Version = %d, want 1", hooksFile.Version)
	}

	// Verify commands
	assertEntryCommand(t, hooksFile.Hooks.Stop, "entire hooks cursor stop")
	assertEntryCommand(t, hooksFile.Hooks.SessionStart, "entire hooks cursor session-start")
	assertEntryCommand(t, hooksFile.Hooks.BeforeSubmitPrompt, "entire hooks cursor before-submit-prompt")

	// Verify matchers on tool hooks
	assertEntryWithMatcher(t, hooksFile.Hooks.PreToolUse, "Task", "entire hooks cursor pre-task")
	assertEntryWithMatcher(t, hooksFile.Hooks.PostToolUse, "Task", "entire hooks cursor post-task")
	assertEntryWithMatcher(t, hooksFile.Hooks.PostToolUse, "TodoWrite", "entire hooks cursor post-todo")
}

func TestInstallHooks_Idempotent(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &CursorAgent{}

	// First install
	count1, err := ag.InstallHooks(false, false)
	if err != nil {
		t.Fatalf("first InstallHooks() error = %v", err)
	}
	if count1 != 7 {
		t.Errorf("first InstallHooks() count = %d, want 7", count1)
	}

	// Second install
	count2, err := ag.InstallHooks(false, false)
	if err != nil {
		t.Fatalf("second InstallHooks() error = %v", err)
	}
	if count2 != 0 {
		t.Errorf("second InstallHooks() count = %d, want 0 (already installed)", count2)
	}

	// Verify no duplicates
	hooksFile := readHooksFile(t, tempDir)
	if len(hooksFile.Hooks.Stop) != 1 {
		t.Errorf("Stop hooks = %d after double install, want 1", len(hooksFile.Hooks.Stop))
	}
}

func TestAreHooksInstalled_NotInstalled(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &CursorAgent{}
	if ag.AreHooksInstalled() {
		t.Error("AreHooksInstalled() = true, want false (no hooks.json)")
	}
}

func TestAreHooksInstalled_AfterInstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &CursorAgent{}

	_, err := ag.InstallHooks(false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	if !ag.AreHooksInstalled() {
		t.Error("AreHooksInstalled() = false, want true")
	}
}

func TestUninstallHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &CursorAgent{}

	// Install
	_, err := ag.InstallHooks(false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}
	if !ag.AreHooksInstalled() {
		t.Fatal("hooks should be installed before uninstall")
	}

	// Uninstall
	err = ag.UninstallHooks()
	if err != nil {
		t.Fatalf("UninstallHooks() error = %v", err)
	}

	if ag.AreHooksInstalled() {
		t.Error("AreHooksInstalled() = true after uninstall, want false")
	}
}

func TestUninstallHooks_NoHooksFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &CursorAgent{}

	// Should not error when no hooks file exists
	err := ag.UninstallHooks()
	if err != nil {
		t.Fatalf("UninstallHooks() should not error when no hooks file: %v", err)
	}
}

func TestInstallHooks_ForceReinstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &CursorAgent{}

	// Install normally
	_, err := ag.InstallHooks(false, false)
	if err != nil {
		t.Fatalf("first InstallHooks() error = %v", err)
	}

	// Force reinstall
	count, err := ag.InstallHooks(false, true)
	if err != nil {
		t.Fatalf("force InstallHooks() error = %v", err)
	}
	if count != 7 {
		t.Errorf("force InstallHooks() count = %d, want 7", count)
	}

	// Verify no duplicates
	hooksFile := readHooksFile(t, tempDir)
	if len(hooksFile.Hooks.Stop) != 1 {
		t.Errorf("Stop hooks = %d after force reinstall, want 1", len(hooksFile.Hooks.Stop))
	}
}

func TestInstallHooks_PreservesExistingHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Create hooks file with existing user hooks
	writeHooksFile(t, tempDir, CursorHooksFile{
		Version: 1,
		Hooks: CursorHooks{
			Stop: []CursorHookEntry{
				{Command: "echo user hook"},
			},
			PostToolUse: []CursorHookEntry{
				{Command: "echo file written", Matcher: "Write"},
			},
		},
	})

	ag := &CursorAgent{}
	_, err := ag.InstallHooks(false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	hooksFile := readHooksFile(t, tempDir)

	// Stop should have user hook + entire hook
	if len(hooksFile.Hooks.Stop) != 2 {
		t.Errorf("Stop hooks = %d, want 2 (user + entire)", len(hooksFile.Hooks.Stop))
	}
	assertEntryCommand(t, hooksFile.Hooks.Stop, "echo user hook")
	assertEntryCommand(t, hooksFile.Hooks.Stop, "entire hooks cursor stop")

	// PostToolUse should have user Write hook + Task hook + TodoWrite hook
	if len(hooksFile.Hooks.PostToolUse) != 3 {
		t.Errorf("PostToolUse hooks = %d, want 3 (user Write + Task + TodoWrite)", len(hooksFile.Hooks.PostToolUse))
	}
	assertEntryWithMatcher(t, hooksFile.Hooks.PostToolUse, "Write", "echo file written")
	assertEntryWithMatcher(t, hooksFile.Hooks.PostToolUse, "Task", "entire hooks cursor post-task")
	assertEntryWithMatcher(t, hooksFile.Hooks.PostToolUse, "TodoWrite", "entire hooks cursor post-todo")
}

func TestInstallHooks_LocalDev(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &CursorAgent{}
	_, err := ag.InstallHooks(true, false)
	if err != nil {
		t.Fatalf("InstallHooks(localDev=true) error = %v", err)
	}

	hooksFile := readHooksFile(t, tempDir)
	assertEntryCommand(t, hooksFile.Hooks.Stop, "go run ${CURSOR_PROJECT_DIR}/cmd/entire/main.go hooks cursor stop")
}

// --- Test helpers ---

func readHooksFile(t *testing.T, tempDir string) CursorHooksFile {
	t.Helper()
	hooksPath := filepath.Join(tempDir, ".cursor", HooksFileName)
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read "+HooksFileName+": %v", err)
	}

	var hooksFile CursorHooksFile
	if err := json.Unmarshal(data, &hooksFile); err != nil {
		t.Fatalf("failed to parse "+HooksFileName+": %v", err)
	}
	return hooksFile
}

func writeHooksFile(t *testing.T, tempDir string, hooksFile CursorHooksFile) {
	t.Helper()
	cursorDir := filepath.Join(tempDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}
	data, err := json.MarshalIndent(hooksFile, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal "+HooksFileName+": %v", err)
	}
	hooksPath := filepath.Join(cursorDir, HooksFileName)
	if err := os.WriteFile(hooksPath, data, 0o644); err != nil {
		t.Fatalf("failed to write "+HooksFileName+": %v", err)
	}
}

func assertEntryCommand(t *testing.T, entries []CursorHookEntry, command string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Command == command {
			return
		}
	}
	t.Errorf("hook with command %q not found", command)
}

func assertEntryWithMatcher(t *testing.T, entries []CursorHookEntry, matcher, command string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Matcher == matcher && entry.Command == command {
			return
		}
	}
	t.Errorf("hook with matcher=%q command=%q not found", matcher, command)
}
