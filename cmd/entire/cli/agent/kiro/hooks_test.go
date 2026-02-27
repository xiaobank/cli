package kiro

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// Compile-time check
var _ agent.HookSupport = (*KiroAgent)(nil)

// Note: Hook tests cannot use t.Parallel() because t.Chdir() modifies process state.

func TestInstallHooks_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ag := &KiroAgent{}

	count, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 hooks installed, got %d", count)
	}

	configPath := filepath.Join(dir, ".kiro", "agents", "entire.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "entire hooks kiro") {
		t.Error("config file does not contain 'entire hooks kiro'")
	}
	if !strings.Contains(content, HookNameAgentSpawn) {
		t.Error("config file does not contain agent-spawn hook")
	}
	if !strings.Contains(content, HookNameStop) {
		t.Error("config file does not contain stop hook")
	}
	if strings.Contains(content, "go run") {
		t.Error("config file contains 'go run' in production mode")
	}
}

func TestInstallHooks_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ag := &KiroAgent{}

	count1, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	if count1 != 5 {
		t.Errorf("first install: expected 5, got %d", count1)
	}

	count2, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	if count2 != 0 {
		t.Errorf("second install: expected 0 (idempotent), got %d", count2)
	}
}

func TestInstallHooks_LocalDev(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ag := &KiroAgent{}

	count, err := ag.InstallHooks(context.Background(), true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 hooks installed, got %d", count)
	}

	configPath := filepath.Join(dir, ".kiro", "agents", "entire.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "go run") {
		t.Error("local dev mode: config file should contain 'go run'")
	}
	if !strings.Contains(content, "${KIRO_PROJECT_DIR}") {
		t.Error("local dev mode: config file should contain ${KIRO_PROJECT_DIR}")
	}
}

func TestInstallHooks_ForceReinstall(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ag := &KiroAgent{}

	if _, err := ag.InstallHooks(context.Background(), false, false); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	count, err := ag.InstallHooks(context.Background(), false, true)
	if err != nil {
		t.Fatalf("force install failed: %v", err)
	}
	if count != 5 {
		t.Errorf("force install: expected 5, got %d", count)
	}
}

func TestUninstallHooks(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ag := &KiroAgent{}

	if _, err := ag.InstallHooks(context.Background(), false, false); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if err := ag.UninstallHooks(context.Background()); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	configPath := filepath.Join(dir, ".kiro", "agents", "entire.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("config file still exists after uninstall")
	}
}

func TestUninstallHooks_NoFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ag := &KiroAgent{}

	if err := ag.UninstallHooks(context.Background()); err != nil {
		t.Fatalf("uninstall with no file should not error: %v", err)
	}
}

func TestAreHooksInstalled(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ag := &KiroAgent{}

	if ag.AreHooksInstalled(context.Background()) {
		t.Error("hooks should not be installed initially")
	}

	if _, err := ag.InstallHooks(context.Background(), false, false); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if !ag.AreHooksInstalled(context.Background()) {
		t.Error("hooks should be installed after InstallHooks")
	}

	if err := ag.UninstallHooks(context.Background()); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	if ag.AreHooksInstalled(context.Background()) {
		t.Error("hooks should not be installed after UninstallHooks")
	}
}

func TestInstallHooks_JSONStructure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	ag := &KiroAgent{}

	if _, err := ag.InstallHooks(context.Background(), false, false); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	configPath := filepath.Join(dir, ".kiro", "agents", "entire.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var config kiroHookConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	if len(config.AgentSpawn) != 1 {
		t.Errorf("expected 1 agentSpawn hook, got %d", len(config.AgentSpawn))
	}
	if len(config.UserPromptSubmit) != 1 {
		t.Errorf("expected 1 userPromptSubmit hook, got %d", len(config.UserPromptSubmit))
	}
	if len(config.PreToolUse) != 1 {
		t.Errorf("expected 1 preToolUse hook, got %d", len(config.PreToolUse))
	}
	if len(config.PostToolUse) != 1 {
		t.Errorf("expected 1 postToolUse hook, got %d", len(config.PostToolUse))
	}
	if len(config.Stop) != 1 {
		t.Errorf("expected 1 stop hook, got %d", len(config.Stop))
	}

	// Verify command format
	if config.AgentSpawn[0].Command != "entire hooks kiro agent-spawn" {
		t.Errorf("unexpected agentSpawn command: %q", config.AgentSpawn[0].Command)
	}
	if config.Stop[0].Command != "entire hooks kiro stop" {
		t.Errorf("unexpected stop command: %q", config.Stop[0].Command)
	}
}

func TestGetSupportedHooks(t *testing.T) {
	t.Parallel()

	ag := &KiroAgent{}
	hooks := ag.GetSupportedHooks()

	if len(hooks) != 5 {
		t.Errorf("expected 5 supported hooks, got %d", len(hooks))
	}

	hookSet := make(map[agent.HookType]bool)
	for _, h := range hooks {
		hookSet[h] = true
	}

	expected := []agent.HookType{
		agent.HookSessionStart,
		agent.HookUserPromptSubmit,
		agent.HookStop,
		agent.HookPreToolUse,
		agent.HookPostToolUse,
	}
	for _, h := range expected {
		if !hookSet[h] {
			t.Errorf("missing expected hook type: %v", h)
		}
	}
}
