package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallHooks_FreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ENTIRE_REPO_ROOT", tempDir)

	// Call the install handler directly by simulating os.Args
	oldArgs := os.Args
	os.Args = []string{"entire-agent-cursor", "install-hooks"}
	defer func() { os.Args = oldArgs }()

	err := cmdInstallHooks()
	if err != nil {
		t.Fatalf("cmdInstallHooks() error = %v", err)
	}

	hf := readHooksFile(t, tempDir)

	if len(hf.Hooks.SessionStart) != 1 {
		t.Errorf("SessionStart hooks = %d, want 1", len(hf.Hooks.SessionStart))
	}
	if len(hf.Hooks.Stop) != 1 {
		t.Errorf("Stop hooks = %d, want 1", len(hf.Hooks.Stop))
	}
	if len(hf.Hooks.SubagentStart) != 1 {
		t.Errorf("SubagentStart hooks = %d, want 1", len(hf.Hooks.SubagentStart))
	}
	if hf.Version != 1 {
		t.Errorf("Version = %d, want 1", hf.Version)
	}

	assertEntryCommand(t, hf.Hooks.Stop, "entire hooks cursor stop")
	assertEntryCommand(t, hf.Hooks.SessionStart, "entire hooks cursor session-start")
}

func TestAreHooksInstalled_NotInstalled(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ENTIRE_REPO_ROOT", tempDir)

	// Redirect stdout to capture JSON
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	err = cmdAreHooksInstalled()

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("cmdAreHooksInstalled() error = %v", err)
	}

	var resp struct {
		Installed bool `json:"installed"`
	}
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Installed {
		t.Error("expected installed=false when no hooks.json exists")
	}
}

func TestInstallHooks_PreservesExistingHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ENTIRE_REPO_ROOT", tempDir)

	writeHooksFile(t, tempDir, cursorHooksFile{
		Version: 1,
		Hooks: cursorHooks{
			Stop: []cursorHookEntry{
				{Command: "echo user hook"},
			},
		},
	})

	oldArgs := os.Args
	os.Args = []string{"entire-agent-cursor", "install-hooks"}
	defer func() { os.Args = oldArgs }()

	err := cmdInstallHooks()
	if err != nil {
		t.Fatalf("cmdInstallHooks() error = %v", err)
	}

	hf := readHooksFile(t, tempDir)
	if len(hf.Hooks.Stop) != 2 {
		t.Errorf("Stop hooks = %d, want 2 (user + entire)", len(hf.Hooks.Stop))
	}
	assertEntryCommand(t, hf.Hooks.Stop, "echo user hook")
	assertEntryCommand(t, hf.Hooks.Stop, "entire hooks cursor stop")
}

func TestUninstallHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ENTIRE_REPO_ROOT", tempDir)

	// Install first
	oldArgs := os.Args
	os.Args = []string{"entire-agent-cursor", "install-hooks"}
	err := cmdInstallHooks()
	if err != nil {
		t.Fatalf("cmdInstallHooks() error = %v", err)
	}

	// Uninstall
	os.Args = []string{"entire-agent-cursor", "uninstall-hooks"}
	err = cmdUninstallHooks()
	os.Args = oldArgs
	if err != nil {
		t.Fatalf("cmdUninstallHooks() error = %v", err)
	}

	// Verify hooks file exists but has no Entire hooks
	hf := readHooksFile(t, tempDir)
	if hasEntireHook(hf.Hooks.SessionStart) ||
		hasEntireHook(hf.Hooks.Stop) ||
		hasEntireHook(hf.Hooks.SubagentStart) {
		t.Error("Entire hooks should be removed after uninstall")
	}
}

func TestInstallHooks_PreservesUnknownFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ENTIRE_REPO_ROOT", tempDir)

	existingJSON := `{
  "version": 1,
  "cursorSettings": {"theme": "dark"},
  "hooks": {
    "stop": [{"command": "echo user stop"}],
    "onNotification": [{"command": "echo notify", "filter": "error"}]
  }
}`
	cursorDir := filepath.Join(tempDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, hooksFileName), []byte(existingJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"entire-agent-cursor", "install-hooks"}
	defer func() { os.Args = oldArgs }()

	err := cmdInstallHooks()
	if err != nil {
		t.Fatalf("cmdInstallHooks() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cursorDir, hooksFileName))
	if err != nil {
		t.Fatal(err)
	}

	var rawFile map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawFile); err != nil {
		t.Fatal(err)
	}

	if _, ok := rawFile["cursorSettings"]; !ok {
		t.Error("unknown top-level field 'cursorSettings' was dropped")
	}

	var rawHooks map[string]json.RawMessage
	if err := json.Unmarshal(rawFile["hooks"], &rawHooks); err != nil {
		t.Fatal(err)
	}

	if _, ok := rawHooks["onNotification"]; !ok {
		t.Error("unknown hook type 'onNotification' was dropped")
	}
}

// --- Test helpers ---

func readHooksFile(t *testing.T, tempDir string) cursorHooksFile {
	t.Helper()
	hooksPath := filepath.Join(tempDir, ".cursor", hooksFileName)
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", hooksFileName, err)
	}

	var hf cursorHooksFile
	if err := json.Unmarshal(data, &hf); err != nil {
		t.Fatalf("failed to parse %s: %v", hooksFileName, err)
	}
	return hf
}

func writeHooksFile(t *testing.T, tempDir string, hf cursorHooksFile) {
	t.Helper()
	cursorDir := filepath.Join(tempDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("failed to create .cursor dir: %v", err)
	}
	data, err := json.MarshalIndent(hf, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal %s: %v", hooksFileName, err)
	}
	hooksPath := filepath.Join(cursorDir, hooksFileName)
	if err := os.WriteFile(hooksPath, data, 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", hooksFileName, err)
	}
}

func assertEntryCommand(t *testing.T, entries []cursorHookEntry, command string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Command == command {
			return
		}
	}
	t.Errorf("hook with command %q not found", command)
}
