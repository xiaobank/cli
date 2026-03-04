package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/jsonutil"
)

const hooksFileName = "hooks.json"

// entireHookPrefixes are command prefixes that identify Entire hooks.
var entireHookPrefixes = []string{
	"entire ",
	"go run ${CURSOR_PROJECT_DIR}/cmd/entire/main.go ",
}

// Hook name constants.
const (
	hookNameSessionStart       = "session-start"
	hookNameSessionEnd         = "session-end"
	hookNameBeforeSubmitPrompt = "before-submit-prompt"
	hookNameStop               = "stop"
	hookNamePreCompact         = "pre-compact"
	hookNameSubagentStart      = "subagent-start"
	hookNameSubagentStop       = "subagent-stop"
)

// cmdInstallHooks installs Cursor hooks in .cursor/hooks.json.
func cmdInstallHooks(localDev, force bool) error {
	repoRoot := os.Getenv("ENTIRE_REPO_ROOT")
	if repoRoot == "" {
		repoRoot = "."
	}

	hooksPath := filepath.Join(repoRoot, ".cursor", hooksFileName)

	var rawFile map[string]json.RawMessage
	var rawHooks map[string]json.RawMessage

	existingData, readErr := os.ReadFile(hooksPath) //nolint:gosec // path constructed from env + fixed suffix
	if readErr == nil {
		if err := json.Unmarshal(existingData, &rawFile); err != nil {
			return fmt.Errorf("failed to parse existing %s: %w", hooksFileName, err)
		}
		if hooksRaw, ok := rawFile["hooks"]; ok {
			if err := json.Unmarshal(hooksRaw, &rawHooks); err != nil {
				return fmt.Errorf("failed to parse hooks in %s: %w", hooksFileName, err)
			}
		}
		if _, ok := rawFile["version"]; !ok {
			rawFile["version"] = json.RawMessage(`1`)
		}
	} else {
		rawFile = map[string]json.RawMessage{
			"version": json.RawMessage(`1`),
		}
	}

	if rawHooks == nil {
		rawHooks = make(map[string]json.RawMessage)
	}

	var sessionStart, sessionEnd, beforeSubmitPrompt, stop, preCompact, subagentStart, subagentStop []CursorHookEntry
	parseCursorHookType(rawHooks, "sessionStart", &sessionStart)
	parseCursorHookType(rawHooks, "sessionEnd", &sessionEnd)
	parseCursorHookType(rawHooks, "beforeSubmitPrompt", &beforeSubmitPrompt)
	parseCursorHookType(rawHooks, "stop", &stop)
	parseCursorHookType(rawHooks, "preCompact", &preCompact)
	parseCursorHookType(rawHooks, "subagentStart", &subagentStart)
	parseCursorHookType(rawHooks, "subagentStop", &subagentStop)

	if force {
		sessionStart = removeEntireHooks(sessionStart)
		sessionEnd = removeEntireHooks(sessionEnd)
		beforeSubmitPrompt = removeEntireHooks(beforeSubmitPrompt)
		stop = removeEntireHooks(stop)
		preCompact = removeEntireHooks(preCompact)
		subagentStart = removeEntireHooks(subagentStart)
		subagentStop = removeEntireHooks(subagentStop)
	}

	var cmdPrefix string
	if localDev {
		cmdPrefix = "go run ${CURSOR_PROJECT_DIR}/cmd/entire/main.go hooks cursor "
	} else {
		cmdPrefix = "entire hooks cursor "
	}

	cmds := map[string]*[]CursorHookEntry{
		cmdPrefix + hookNameSessionStart:       &sessionStart,
		cmdPrefix + hookNameSessionEnd:         &sessionEnd,
		cmdPrefix + hookNameBeforeSubmitPrompt: &beforeSubmitPrompt,
		cmdPrefix + hookNameStop:               &stop,
		cmdPrefix + hookNamePreCompact:         &preCompact,
		cmdPrefix + hookNameSubagentStart:      &subagentStart,
		cmdPrefix + hookNameSubagentStop:       &subagentStop,
	}

	count := 0
	for cmd, entries := range cmds {
		if !hookCommandExists(*entries, cmd) {
			*entries = append(*entries, CursorHookEntry{Command: cmd})
			count++
		}
	}

	if count == 0 {
		return writeJSON(map[string]int{"hooks_installed": 0})
	}

	marshalCursorHookType(rawHooks, "sessionStart", sessionStart)
	marshalCursorHookType(rawHooks, "sessionEnd", sessionEnd)
	marshalCursorHookType(rawHooks, "beforeSubmitPrompt", beforeSubmitPrompt)
	marshalCursorHookType(rawHooks, "stop", stop)
	marshalCursorHookType(rawHooks, "preCompact", preCompact)
	marshalCursorHookType(rawHooks, "subagentStart", subagentStart)
	marshalCursorHookType(rawHooks, "subagentStop", subagentStop)

	hooksJSON, err := json.Marshal(rawHooks)
	if err != nil {
		return fmt.Errorf("failed to marshal hooks: %w", err)
	}
	rawFile["hooks"] = hooksJSON

	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o750); err != nil { //nolint:gosec // path constructed from env + fixed suffix
		return fmt.Errorf("failed to create .cursor directory: %w", err)
	}

	output, err := jsonutil.MarshalIndentWithNewline(rawFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", hooksFileName, err)
	}

	if err := os.WriteFile(hooksPath, output, 0o600); err != nil { //nolint:gosec // path constructed from env + fixed suffix
		return fmt.Errorf("failed to write %s: %w", hooksFileName, err)
	}

	return writeJSON(map[string]int{"hooks_installed": count})
}

// cmdUninstallHooks removes Entire hooks from .cursor/hooks.json.
func cmdUninstallHooks() error {
	repoRoot := os.Getenv("ENTIRE_REPO_ROOT")
	if repoRoot == "" {
		repoRoot = "."
	}

	hooksPath := filepath.Join(repoRoot, ".cursor", hooksFileName)
	data, err := os.ReadFile(hooksPath) //nolint:gosec // path constructed from env + fixed suffix
	if err != nil {
		return nil //nolint:nilerr // No hooks file means nothing to uninstall
	}

	var rawFile map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawFile); err != nil {
		return fmt.Errorf("failed to parse %s: %w", hooksFileName, err)
	}

	var rawHooks map[string]json.RawMessage
	if hooksRaw, ok := rawFile["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &rawHooks); err != nil {
			return fmt.Errorf("failed to parse hooks in %s: %w", hooksFileName, err)
		}
	}
	if rawHooks == nil {
		rawHooks = make(map[string]json.RawMessage)
	}

	var sessionStart, sessionEnd, beforeSubmitPrompt, stop, preCompact, subagentStart, subagentStop []CursorHookEntry
	parseCursorHookType(rawHooks, "sessionStart", &sessionStart)
	parseCursorHookType(rawHooks, "sessionEnd", &sessionEnd)
	parseCursorHookType(rawHooks, "beforeSubmitPrompt", &beforeSubmitPrompt)
	parseCursorHookType(rawHooks, "stop", &stop)
	parseCursorHookType(rawHooks, "preCompact", &preCompact)
	parseCursorHookType(rawHooks, "subagentStart", &subagentStart)
	parseCursorHookType(rawHooks, "subagentStop", &subagentStop)

	sessionStart = removeEntireHooks(sessionStart)
	sessionEnd = removeEntireHooks(sessionEnd)
	beforeSubmitPrompt = removeEntireHooks(beforeSubmitPrompt)
	stop = removeEntireHooks(stop)
	preCompact = removeEntireHooks(preCompact)
	subagentStart = removeEntireHooks(subagentStart)
	subagentStop = removeEntireHooks(subagentStop)

	marshalCursorHookType(rawHooks, "sessionStart", sessionStart)
	marshalCursorHookType(rawHooks, "sessionEnd", sessionEnd)
	marshalCursorHookType(rawHooks, "beforeSubmitPrompt", beforeSubmitPrompt)
	marshalCursorHookType(rawHooks, "stop", stop)
	marshalCursorHookType(rawHooks, "preCompact", preCompact)
	marshalCursorHookType(rawHooks, "subagentStart", subagentStart)
	marshalCursorHookType(rawHooks, "subagentStop", subagentStop)

	if len(rawHooks) > 0 {
		hooksJSON, err := json.Marshal(rawHooks)
		if err != nil {
			return fmt.Errorf("failed to marshal hooks: %w", err)
		}
		rawFile["hooks"] = hooksJSON
	} else {
		delete(rawFile, "hooks")
	}

	output, err := jsonutil.MarshalIndentWithNewline(rawFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", hooksFileName, err)
	}

	if err := os.WriteFile(hooksPath, output, 0o600); err != nil { //nolint:gosec // path constructed from env + fixed suffix
		return fmt.Errorf("failed to write %s: %w", hooksFileName, err)
	}
	return nil
}

// cmdAreHooksInstalled checks if Entire hooks are installed.
func cmdAreHooksInstalled() error {
	repoRoot := os.Getenv("ENTIRE_REPO_ROOT")
	if repoRoot == "" {
		repoRoot = "."
	}

	hooksPath := filepath.Join(repoRoot, ".cursor", hooksFileName)
	data, err := os.ReadFile(hooksPath) //nolint:gosec // path constructed from env + fixed suffix
	if err != nil {
		return writeJSON(map[string]bool{"installed": false})
	}

	var hooksFile CursorHooksFile
	if err := json.Unmarshal(data, &hooksFile); err != nil {
		return writeJSON(map[string]bool{"installed": false})
	}

	installed := hasEntireHook(hooksFile.Hooks.SessionStart) ||
		hasEntireHook(hooksFile.Hooks.SessionEnd) ||
		hasEntireHook(hooksFile.Hooks.BeforeSubmitPrompt) ||
		hasEntireHook(hooksFile.Hooks.Stop) ||
		hasEntireHook(hooksFile.Hooks.PreCompact) ||
		hasEntireHook(hooksFile.Hooks.SubagentStart) ||
		hasEntireHook(hooksFile.Hooks.SubagentStop)

	return writeJSON(map[string]bool{"installed": installed})
}

// parseCursorHookType parses a specific hook type from rawHooks into the target slice.
func parseCursorHookType(rawHooks map[string]json.RawMessage, hookType string, target *[]CursorHookEntry) {
	if data, ok := rawHooks[hookType]; ok {
		//nolint:errcheck,gosec // Intentionally ignoring parse errors
		json.Unmarshal(data, target)
	}
}

// marshalCursorHookType marshals a hook type back into rawHooks.
func marshalCursorHookType(rawHooks map[string]json.RawMessage, hookType string, entries []CursorHookEntry) {
	if len(entries) == 0 {
		delete(rawHooks, hookType)
		return
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return
	}
	rawHooks[hookType] = data
}

func hookCommandExists(entries []CursorHookEntry, command string) bool {
	for _, entry := range entries {
		if entry.Command == command {
			return true
		}
	}
	return false
}

func isEntireHook(command string) bool {
	for _, prefix := range entireHookPrefixes {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func hasEntireHook(entries []CursorHookEntry) bool {
	for _, entry := range entries {
		if isEntireHook(entry.Command) {
			return true
		}
	}
	return false
}

func removeEntireHooks(entries []CursorHookEntry) []CursorHookEntry {
	result := make([]CursorHookEntry, 0, len(entries))
	for _, entry := range entries {
		if !isEntireHook(entry.Command) {
			result = append(result, entry)
		}
	}
	return result
}
