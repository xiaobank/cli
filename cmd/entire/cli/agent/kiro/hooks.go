package kiro

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/jsonutil"
	"github.com/entireio/cli/cmd/entire/cli/paths"
)

// Compile-time interface assertion
var _ agent.HookSupport = (*KiroAgent)(nil)

const (
	// agentsDirName is the directory under .kiro/ where agent configs live
	agentsDirName = "agents"

	// configFileName is the name of our hook config file
	configFileName = "entire.json"

	// entireMarker is a string present in the config to identify it as Entire's
	entireMarker = "entire hooks kiro"
)

// entireHookPrefixes are command prefixes that identify Entire hooks
var entireHookPrefixes = []string{
	"entire ",
	"go run ${KIRO_PROJECT_DIR}/cmd/entire/main.go ",
}

// kiroHookConfig represents the .kiro/agents/entire.json file structure.
// Each key is a Kiro hook event name (camelCase), and the value is an array of commands.
type kiroHookConfig struct {
	AgentSpawn       []hookCommand `json:"agentSpawn,omitempty"`
	UserPromptSubmit []hookCommand `json:"userPromptSubmit,omitempty"`
	PreToolUse       []hookCommand `json:"preToolUse,omitempty"`
	PostToolUse      []hookCommand `json:"postToolUse,omitempty"`
	Stop             []hookCommand `json:"stop,omitempty"`
}

// hookCommand represents a single hook command entry.
type hookCommand struct {
	Command string `json:"command"`
}

// getConfigPath returns the absolute path to the hook config file.
func getConfigPath(ctx context.Context) (string, error) {
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		//nolint:forbidigo // Intentional fallback when WorktreeRoot() fails (tests run outside git repos)
		repoRoot, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
	}
	return filepath.Join(repoRoot, ".kiro", agentsDirName, configFileName), nil
}

// InstallHooks writes the Entire hook config to .kiro/agents/entire.json.
// Returns the number of hooks installed, 0 if already present (idempotent).
func (k *KiroAgent) InstallHooks(ctx context.Context, localDev bool, force bool) (int, error) {
	configPath, err := getConfigPath(ctx)
	if err != nil {
		return 0, err
	}

	// Check if already installed (idempotent) unless force
	if !force {
		if data, err := os.ReadFile(configPath); err == nil { //nolint:gosec // Path constructed from repo root
			if strings.Contains(string(data), entireMarker) {
				return 0, nil
			}
		}
	}

	var cmdPrefix string
	if localDev {
		cmdPrefix = "go run ${KIRO_PROJECT_DIR}/cmd/entire/main.go hooks kiro "
	} else {
		cmdPrefix = "entire hooks kiro "
	}

	config := kiroHookConfig{
		AgentSpawn:       []hookCommand{{Command: cmdPrefix + HookNameAgentSpawn}},
		UserPromptSubmit: []hookCommand{{Command: cmdPrefix + HookNameUserPromptSubmit}},
		PreToolUse:       []hookCommand{{Command: cmdPrefix + HookNamePreToolUse}},
		PostToolUse:      []hookCommand{{Command: cmdPrefix + HookNamePostToolUse}},
		Stop:             []hookCommand{{Command: cmdPrefix + HookNameStop}},
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return 0, fmt.Errorf("failed to create .kiro/agents directory: %w", err)
	}

	output, err := jsonutil.MarshalIndentWithNewline(config, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("failed to marshal hook config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0o600); err != nil {
		return 0, fmt.Errorf("failed to write hook config: %w", err)
	}

	return 5, nil
}

// UninstallHooks removes the Entire hook config file.
func (k *KiroAgent) UninstallHooks(ctx context.Context) error {
	configPath, err := getConfigPath(ctx)
	if err != nil {
		return err
	}

	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove hook config: %w", err)
	}

	return nil
}

// AreHooksInstalled checks if the Entire hook config file exists and contains our hooks.
func (k *KiroAgent) AreHooksInstalled(ctx context.Context) bool {
	configPath, err := getConfigPath(ctx)
	if err != nil {
		return false
	}

	data, err := os.ReadFile(configPath) //nolint:gosec // Path constructed from repo root
	if err != nil {
		return false
	}

	// Check for Entire command prefix in the config
	content := string(data)
	for _, prefix := range entireHookPrefixes {
		if strings.Contains(content, prefix) {
			return true
		}
	}

	// Also check by parsing the JSON structure
	var config kiroHookConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}

	return hasEntireCommand(config.AgentSpawn) ||
		hasEntireCommand(config.UserPromptSubmit) ||
		hasEntireCommand(config.PreToolUse) ||
		hasEntireCommand(config.PostToolUse) ||
		hasEntireCommand(config.Stop)
}

// GetSupportedHooks returns the normalized lifecycle events this agent supports.
func (k *KiroAgent) GetSupportedHooks() []agent.HookType {
	return []agent.HookType{
		agent.HookSessionStart,
		agent.HookUserPromptSubmit,
		agent.HookStop,
		agent.HookPreToolUse,
		agent.HookPostToolUse,
	}
}

// hasEntireCommand checks if any command in the list starts with an Entire prefix.
func hasEntireCommand(commands []hookCommand) bool {
	for _, cmd := range commands {
		for _, prefix := range entireHookPrefixes {
			if strings.HasPrefix(cmd.Command, prefix) {
				return true
			}
		}
	}
	return false
}
