package strategy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/settings"
)

// Hook marker used to identify Entire CLI hooks
const entireHookMarker = "Entire CLI hooks"

const backupSuffix = ".pre-entire"
const chainComment = "# Chain: run pre-existing hook"

// Husky injection markers
const huskyBeginMarker = "# BEGIN entire"
const huskyEndMarker = "# END entire"

// gitHookNames are the git hooks managed by Entire CLI
var gitHookNames = []string{"prepare-commit-msg", "commit-msg", "post-commit", "pre-push"}

// ManagedGitHookNames returns the list of git hooks managed by Entire CLI.
// This is useful for tests that need to manipulate hooks.
func ManagedGitHookNames() []string {
	return gitHookNames
}

// hookSpec defines a git hook's name and content template (without chain call).
type hookSpec struct {
	name    string
	content string
}

// GetGitDir returns the actual git directory path by delegating to git itself.
// This handles both regular repositories and worktrees, and inherits git's
// security validation for gitdir references.
func GetGitDir() (string, error) {
	return getGitDirInPath(".")
}

// GetHooksDir returns the active hooks directory path.
// This respects core.hooksPath and correctly resolves to the common hooks
// directory when called from a linked worktree.
func GetHooksDir() (string, error) {
	return getHooksDirInPath(".")
}

// getGitDirInPath returns the git directory for a repository at the given path.
// It delegates to `git rev-parse --git-dir` to leverage git's own validation.
func getGitDirInPath(dir string) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("not a git repository")
	}

	gitDir := strings.TrimSpace(string(output))

	// git rev-parse --git-dir returns relative paths from the working directory,
	// so we need to make it absolute if it isn't already
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}

	return filepath.Clean(gitDir), nil
}

// getHooksDirInPath returns the active hooks directory for a repository at the given path.
// It delegates to `git rev-parse --git-path hooks` so Git resolves:
// - linked-worktree common hooks directory
// - core.hooksPath (relative or absolute)
func getHooksDirInPath(dir string) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-path", "hooks")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("not a git repository")
	}

	hooksDir := strings.TrimSpace(string(output))
	if !filepath.IsAbs(hooksDir) {
		hooksDir = filepath.Join(dir, hooksDir)
	}

	return filepath.Clean(hooksDir), nil
}

// IsGitHookInstalled checks if all generic Entire CLI hooks are installed.
func IsGitHookInstalled() bool {
	hooksDir, err := GetHooksDir()
	if err != nil {
		return false
	}
	return isGitHookInstalledInHooksDir(hooksDir)
}

// IsGitHookInstalledInDir checks if all Entire CLI hooks are installed in the given repo directory.
// This is useful for tests that need to check hooks without changing the working directory.
func IsGitHookInstalledInDir(repoDir string) bool {
	hooksDir, err := getHooksDirInPath(repoDir)
	if err != nil {
		return false
	}
	return isGitHookInstalledInHooksDir(hooksDir)
}

// isGitHookInstalledInHooksDir checks if all hooks are installed in the given hooks directory.
// When Husky is detected and hooks are installed as injected blocks, it checks the
// user-editable .husky/<hookname> files. Otherwise falls through to standard check.
func isGitHookInstalledInHooksDir(hooksDir string) bool {
	// When Husky is detected, check user-facing hook files for injected blocks
	if huskyUserDir := getHuskyUserHooksDir(hooksDir); huskyUserDir != "" {
		if hasHuskyInjectedHooks(huskyUserDir) {
			for _, hook := range gitHookNames {
				hookPath := filepath.Join(huskyUserDir, hook)
				data, err := os.ReadFile(hookPath) //nolint:gosec // Path is constructed from constants
				if err != nil {
					return false
				}
				if extractEntireBlock(string(data)) == "" {
					return false
				}
			}
			return true
		}
	}

	// Standard check: look for standalone hook files with our marker
	for _, hook := range gitHookNames {
		hookPath := filepath.Join(hooksDir, hook)
		data, err := os.ReadFile(hookPath) //nolint:gosec // Path is constructed from constants
		if err != nil {
			return false
		}
		if !strings.Contains(string(data), entireHookMarker) {
			return false
		}
	}
	return true
}

// buildHookSpecs returns the hook specifications for all managed hooks.
func buildHookSpecs(cmdPrefix string) []hookSpec {
	return []hookSpec{
		{
			name: "prepare-commit-msg",
			content: fmt.Sprintf(`#!/bin/sh
# %s
%s hooks git prepare-commit-msg "$1" "$2" 2>/dev/null || true
`, entireHookMarker, cmdPrefix),
		},
		{
			name: "commit-msg",
			content: fmt.Sprintf(`#!/bin/sh
# %s
# Commit-msg hook: strip trailer if no user content (allows aborting empty commits)
%s hooks git commit-msg "$1" || exit 1
`, entireHookMarker, cmdPrefix),
		},
		{
			name: "post-commit",
			content: fmt.Sprintf(`#!/bin/sh
# %s
# Post-commit hook: condense session data if commit has Entire-Checkpoint trailer
%s hooks git post-commit 2>/dev/null || true
`, entireHookMarker, cmdPrefix),
		},
		{
			name: "pre-push",
			content: fmt.Sprintf(`#!/bin/sh
# %s
# Pre-push hook: push session logs alongside user's push
# $1 is the remote name (e.g., "origin")
%s hooks git pre-push "$1" || true
`, entireHookMarker, cmdPrefix),
		},
	}
}

// InstallGitHook installs generic git hooks that delegate to `entire hook` commands.
// These hooks work with any strategy - the strategy is determined at runtime.
// If silent is true, no output is printed (except backup notifications, which always print).
// Returns the number of hooks that were installed (0 if all already up to date).
//
// When Husky v9 is detected (core.hooksPath points to .husky/_), hooks are injected
// into the user-editable .husky/<hookname> files instead of writing standalone hooks
// into .husky/_/. This ensures hooks survive `npm install` which regenerates .husky/_/.
func InstallGitHook(silent bool) (int, error) {
	hooksDir, err := GetHooksDir()
	if err != nil {
		return 0, err
	}

	// Determine command prefix based on local_dev setting
	var cmdPrefix string
	if isLocalDev() {
		cmdPrefix = "go run ./cmd/entire/main.go"
	} else {
		cmdPrefix = "entire"
	}

	// Check for Husky v9: core.hooksPath resolves to .husky/_
	if huskyUserDir := getHuskyUserHooksDir(hooksDir); huskyUserDir != "" {
		return installHuskyHooks(hooksDir, huskyUserDir, cmdPrefix, silent)
	}

	return installStandardHooks(hooksDir, cmdPrefix, silent)
}

// installStandardHooks installs standalone git hooks into the hooks directory.
func installStandardHooks(hooksDir, cmdPrefix string, silent bool) (int, error) {
	if err := os.MkdirAll(hooksDir, 0o755); err != nil { //nolint:gosec // Git hooks require executable permissions
		return 0, fmt.Errorf("failed to create hooks directory: %w", err)
	}

	specs := buildHookSpecs(cmdPrefix)
	installedCount := 0

	for _, spec := range specs {
		hookPath := filepath.Join(hooksDir, spec.name)
		backupPath := hookPath + backupSuffix
		backupExists := fileExists(backupPath)

		// Back up existing non-Entire hooks
		existing, existingErr := os.ReadFile(hookPath) //nolint:gosec // path is controlled
		if existingErr == nil && !strings.Contains(string(existing), entireHookMarker) {
			if !backupExists {
				if err := os.Rename(hookPath, backupPath); err != nil {
					return installedCount, fmt.Errorf("failed to back up %s: %w", spec.name, err)
				}
				fmt.Fprintf(os.Stderr, "[entire] Backed up existing %s to %s%s\n", spec.name, spec.name, backupSuffix)
			} else {
				fmt.Fprintf(os.Stderr, "[entire] Warning: replacing %s (backup %s%s already exists from a previous install)\n", spec.name, spec.name, backupSuffix)
			}
			backupExists = true
		}

		// Chain to backup if one exists
		content := spec.content
		if backupExists {
			content = generateChainedContent(spec.content, spec.name)
		}

		written, err := writeHookFile(hookPath, content)
		if err != nil {
			return installedCount, fmt.Errorf("failed to install %s hook: %w", spec.name, err)
		}
		if written {
			installedCount++
		}
	}

	if !silent {
		fmt.Println("✓ Installed git hooks (prepare-commit-msg, commit-msg, post-commit, pre-push)")
		fmt.Println("  Hooks delegate to the current strategy at runtime")
	}

	return installedCount, nil
}

// writeHookFile writes a hook file if it doesn't exist or has different content.
// Returns true if the file was written, false if it already had the same content.
func writeHookFile(path, content string) (bool, error) {
	// Check if file already exists with same content
	existing, err := os.ReadFile(path) //nolint:gosec // path is controlled
	if err == nil && string(existing) == content {
		return false, nil // Already up to date
	}

	// Git hooks must be executable (0o755)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil { //nolint:gosec // Git hooks require executable permissions
		return false, fmt.Errorf("failed to write hook file %s: %w", path, err)
	}
	return true, nil
}

// RemoveGitHook removes all Entire CLI git hooks from the repository.
// For standard hooks: removes hook files and restores .pre-entire backups.
// For Husky hooks: strips BEGIN/END entire blocks from .husky/<hookname> files.
// Returns the number of hooks removed.
func RemoveGitHook() (int, error) {
	hooksDir, err := GetHooksDir()
	if err != nil {
		return 0, err
	}

	// When Husky is detected and hooks are actually installed as Husky injections,
	// strip blocks from user-facing hook files.
	if huskyUserDir := getHuskyUserHooksDir(hooksDir); huskyUserDir != "" {
		if hasHuskyInjectedHooks(huskyUserDir) {
			return removeHuskyHooks(huskyUserDir)
		}
	}

	return removeStandardHooks(hooksDir)
}

// removeStandardHooks removes standalone Entire hook files and restores backups.
func removeStandardHooks(hooksDir string) (int, error) {
	removed := 0
	var removeErrors []string

	for _, hook := range gitHookNames {
		hookPath := filepath.Join(hooksDir, hook)
		backupPath := hookPath + backupSuffix

		// Remove the hook if it contains our marker
		data, err := os.ReadFile(hookPath) //nolint:gosec // path is controlled
		hookIsOurs := err == nil && strings.Contains(string(data), entireHookMarker)
		hookExists := err == nil

		if hookIsOurs {
			if err := os.Remove(hookPath); err != nil {
				removeErrors = append(removeErrors, fmt.Sprintf("%s: %v", hook, err))
				continue
			}
			removed++
		}

		// Restore .pre-entire backup if it exists
		if fileExists(backupPath) {
			if hookExists && !hookIsOurs {
				// A non-Entire hook is present — don't overwrite it with the backup
				fmt.Fprintf(os.Stderr, "[entire] Warning: %s was modified since install; backup %s%s left in place\n", hook, hook, backupSuffix)
			} else {
				if err := os.Rename(backupPath, hookPath); err != nil {
					removeErrors = append(removeErrors, fmt.Sprintf("restore %s%s: %v", hook, backupSuffix, err))
				}
			}
		}
	}

	if len(removeErrors) > 0 {
		return removed, fmt.Errorf("failed to remove hooks: %s", strings.Join(removeErrors, "; "))
	}
	return removed, nil
}

// generateChainedContent appends a chain call to the base hook content,
// so the pre-existing hook (backed up to .pre-entire) is called after our hook.
func generateChainedContent(baseContent, hookName string) string {
	return baseContent + fmt.Sprintf(`%s
_entire_hook_dir="$(dirname "$0")"
if [ -x "$_entire_hook_dir/%s%s" ]; then
    "$_entire_hook_dir/%s%s" "$@"
fi
`, chainComment, hookName, backupSuffix, hookName, backupSuffix)
}

// isHuskyHooksPath returns true if the given hooks directory is managed by Husky v9.
// Husky v9 sets core.hooksPath to .husky/_, so we detect it by path suffix.
func isHuskyHooksPath(hooksDir string) bool {
	cleaned := filepath.Clean(hooksDir)
	return filepath.Base(filepath.Dir(cleaned)) == ".husky" && filepath.Base(cleaned) == "_"
}

// getHuskyUserHooksDir returns the Husky user-editable hooks directory (.husky/)
// from the Husky generated hooks directory (.husky/_), or empty string if not a Husky path
// or if the .husky/ directory doesn't exist on disk.
func getHuskyUserHooksDir(hooksDir string) string {
	if !isHuskyHooksPath(hooksDir) {
		return ""
	}
	// .husky/_  →  .husky/
	userDir := filepath.Dir(filepath.Clean(hooksDir))
	if !fileExists(userDir) {
		return ""
	}
	return userDir
}

// hasHuskyInjectedHooks returns true if any Husky user hook files contain an Entire block.
func hasHuskyInjectedHooks(huskyUserDir string) bool {
	for _, hook := range gitHookNames {
		hookPath := filepath.Join(huskyUserDir, hook)
		data, err := os.ReadFile(hookPath) //nolint:gosec // path is controlled
		if err != nil {
			continue
		}
		if extractEntireBlock(string(data)) != "" {
			return true
		}
	}
	return false
}

// huskyInjectionSpec defines hook content to inject into Husky user-facing hook files.
type huskyInjectionSpec struct {
	name    string
	content string // content between BEGIN/END markers (without markers themselves)
}

// buildHuskyInjectionSpecs returns injection specifications for all managed hooks.
// The content does not include a shebang — Husky v9 user scripts run in the wrapper's shell context.
func buildHuskyInjectionSpecs(cmdPrefix string) []huskyInjectionSpec {
	return []huskyInjectionSpec{
		{
			name:    "prepare-commit-msg",
			content: fmt.Sprintf("# %s\n%s hooks git prepare-commit-msg \"$1\" \"$2\" 2>/dev/null || true", entireHookMarker, cmdPrefix),
		},
		{
			name:    "commit-msg",
			content: fmt.Sprintf("# %s\n# Commit-msg hook: strip trailer if no user content (allows aborting empty commits)\n%s hooks git commit-msg \"$1\" || exit 1", entireHookMarker, cmdPrefix),
		},
		{
			name:    "post-commit",
			content: fmt.Sprintf("# %s\n# Post-commit hook: condense session data if commit has Entire-Checkpoint trailer\n%s hooks git post-commit 2>/dev/null || true", entireHookMarker, cmdPrefix),
		},
		{
			name:    "pre-push",
			content: fmt.Sprintf("# %s\n# Pre-push hook: push session logs alongside user's push\n# $1 is the remote name (e.g., \"origin\")\n%s hooks git pre-push \"$1\" || true", entireHookMarker, cmdPrefix),
		},
	}
}

// extractEntireBlock returns the BEGIN/END entire block from content (inclusive of markers),
// or empty string if not found.
func extractEntireBlock(content string) string {
	beginIdx := strings.Index(content, huskyBeginMarker)
	if beginIdx == -1 {
		return ""
	}
	endIdx := strings.Index(content, huskyEndMarker)
	if endIdx == -1 {
		return ""
	}
	return content[beginIdx : endIdx+len(huskyEndMarker)]
}

// replaceEntireBlock replaces an existing BEGIN/END entire block with newBlock.
// If no existing block is found, returns the original content unchanged.
func replaceEntireBlock(content, newBlock string) string {
	existing := extractEntireBlock(content)
	if existing == "" {
		return content
	}
	return strings.Replace(content, existing, newBlock, 1)
}

// buildEntireBlock wraps injection content in BEGIN/END markers.
func buildEntireBlock(injectionContent string) string {
	return huskyBeginMarker + "\n" + injectionContent + "\n" + huskyEndMarker
}

// injectHuskyHookContent injects or updates an Entire block in a hook file's content.
// If the file has no existing block, the block is appended.
// If the file already has a block, it is replaced.
// Returns the new file content.
func injectHuskyHookContent(existingContent, injectionContent string) string {
	block := buildEntireBlock(injectionContent)

	if extractEntireBlock(existingContent) != "" {
		return replaceEntireBlock(existingContent, block)
	}

	// Append to existing content
	if existingContent == "" {
		return block + "\n"
	}
	// Ensure a newline before the block
	if !strings.HasSuffix(existingContent, "\n") {
		existingContent += "\n"
	}
	return existingContent + block + "\n"
}

// removeEntireBlock strips the BEGIN/END entire block from content.
// Returns the cleaned content with the block removed.
func removeEntireBlock(content string) string {
	existing := extractEntireBlock(content)
	if existing == "" {
		return content
	}
	result := strings.Replace(content, existing, "", 1)
	// Clean up extra blank lines that may result from removal
	result = strings.TrimRight(result, "\n")
	if result != "" {
		result += "\n"
	}
	return result
}

// installHuskyHooks injects Entire hook commands into Husky user-facing hook files (.husky/<hookname>).
// It also cleans up any stale standalone hooks in .husky/_/ from previous installations.
func installHuskyHooks(hooksDir, huskyUserDir, cmdPrefix string, silent bool) (int, error) {
	specs := buildHuskyInjectionSpecs(cmdPrefix)
	installedCount := 0

	for _, spec := range specs {
		userHookPath := filepath.Join(huskyUserDir, spec.name)
		block := buildEntireBlock(spec.content)

		var existingContent string
		data, err := os.ReadFile(userHookPath) //nolint:gosec // path is controlled
		if err == nil {
			existingContent = string(data)
		}

		// Check if already has our block with same content
		if existingBlock := extractEntireBlock(existingContent); existingBlock == block {
			// Already up to date — still clean up stale hooks
			cleanUpStaleHuskyHook(hooksDir, spec.name)
			continue
		}

		newContent := injectHuskyHookContent(existingContent, spec.content)

		if err := os.WriteFile(userHookPath, []byte(newContent), 0o755); err != nil { //nolint:gosec // Hook files need executable permissions
			return installedCount, fmt.Errorf("failed to inject %s into Husky hook: %w", spec.name, err)
		}
		installedCount++

		// Clean up stale standalone hook in .husky/_/ to prevent double execution
		cleanUpStaleHuskyHook(hooksDir, spec.name)
	}

	if !silent {
		fmt.Println("✓ Installed git hooks into Husky user hooks (prepare-commit-msg, commit-msg, post-commit, pre-push)")
		fmt.Println("  Hooks injected into .husky/ hook files (survives npm install)")
	}

	return installedCount, nil
}

// cleanUpStaleHuskyHook removes a stale standalone Entire hook from .husky/_/ if it exists.
func cleanUpStaleHuskyHook(generatedHooksDir, hookName string) {
	hookPath := filepath.Join(generatedHooksDir, hookName)
	data, err := os.ReadFile(hookPath) //nolint:gosec // path is controlled
	if err != nil {
		return
	}
	if strings.Contains(string(data), entireHookMarker) {
		_ = os.Remove(hookPath)
	}
}

// removeHuskyHooks strips Entire blocks from Husky user-facing hook files (.husky/<hookname>).
// If a file becomes empty after removal, it is deleted.
// Returns the number of hooks removed.
func removeHuskyHooks(huskyUserDir string) (int, error) {
	removed := 0
	var removeErrors []string

	for _, hook := range gitHookNames {
		userHookPath := filepath.Join(huskyUserDir, hook)
		data, err := os.ReadFile(userHookPath) //nolint:gosec // path is controlled
		if err != nil {
			continue // File doesn't exist, nothing to remove
		}

		content := string(data)
		if extractEntireBlock(content) == "" {
			continue // No Entire block in this file
		}

		cleaned := removeEntireBlock(content)
		cleaned = strings.TrimSpace(cleaned)

		if cleaned == "" {
			// File is empty after removing our block — delete it
			if err := os.Remove(userHookPath); err != nil {
				removeErrors = append(removeErrors, fmt.Sprintf("%s: %v", hook, err))
				continue
			}
		} else {
			// Write back the file without our block
			if err := os.WriteFile(userHookPath, []byte(cleaned+"\n"), 0o755); err != nil { //nolint:gosec // Hook files need executable permissions
				removeErrors = append(removeErrors, fmt.Sprintf("%s: %v", hook, err))
				continue
			}
		}
		removed++
	}

	if len(removeErrors) > 0 {
		return removed, fmt.Errorf("failed to remove Husky hooks: %s", strings.Join(removeErrors, "; "))
	}
	return removed, nil
}

// isLocalDev reads the local_dev setting from .entire/settings.json
// Works correctly from any subdirectory within the repository.
func isLocalDev() bool {
	s, err := settings.Load()
	if err != nil {
		return false
	}
	return s.LocalDev
}
