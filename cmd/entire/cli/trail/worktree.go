package trail

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/paths"
)

// WorktreeDir is the directory where trail worktrees are created.
const WorktreeDir = ".worktrees"

// WorktreeManager manages git worktrees for trail execution.
type WorktreeManager struct {
	repoRoot string
}

// NewWorktreeManager creates a new worktree manager.
// The repoRoot should be the path to the main git repository.
func NewWorktreeManager(repoRoot string) *WorktreeManager {
	return &WorktreeManager{repoRoot: repoRoot}
}

// WorktreePath returns the path where a trail's worktree should be created.
func (m *WorktreeManager) WorktreePath(id TrailID) string {
	return filepath.Join(m.repoRoot, WorktreeDir, "trail-"+id.String())
}

// Create creates a new git worktree for a trail.
// If the branch doesn't exist, it will be created from baseBranch.
func (m *WorktreeManager) Create(ctx context.Context, id TrailID, branch, baseBranch string) (string, error) {
	worktreePath := m.WorktreePath(id)

	// Ensure the worktrees directory exists
	worktreesDir := filepath.Join(m.repoRoot, WorktreeDir)
	if err := os.MkdirAll(worktreesDir, 0o755); err != nil { //nolint:gosec // worktrees dir needs to be readable by git
		return "", fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		return worktreePath, nil // Already exists
	}

	// Check if the branch exists
	branchExists, err := m.branchExists(ctx, branch)
	if err != nil {
		return "", fmt.Errorf("failed to check branch existence: %w", err)
	}

	if branchExists {
		// Worktree for existing branch
		//nolint:gosec // git command with validated branch names from trail config
		cmd := exec.CommandContext(ctx, "git", "worktree", "add", worktreePath, branch)
		cmd.Dir = m.repoRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to create worktree: %s: %w", strings.TrimSpace(string(output)), err)
		}
	} else {
		// Create new branch from base
		if baseBranch == "" {
			// Get default branch
			baseBranch, err = m.getDefaultBranch(ctx)
			if err != nil {
				return "", fmt.Errorf("failed to determine base branch: %w", err)
			}
		}

		// Create worktree with new branch
		//nolint:gosec // git command with validated branch names from trail config
		cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, worktreePath, baseBranch)
		cmd.Dir = m.repoRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to create worktree with new branch: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}

	return worktreePath, nil
}

// Remove removes a trail's worktree.
func (m *WorktreeManager) Remove(ctx context.Context, id TrailID) error {
	worktreePath := m.WorktreePath(id)

	// Check if worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return nil // Already removed
	}

	// Remove the worktree
	//nolint:gosec // worktreePath is constructed from validated trail ID
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = m.repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove worktree: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// Exists checks if a worktree exists for the given trail.
func (m *WorktreeManager) Exists(id TrailID) bool {
	worktreePath := m.WorktreePath(id)
	_, err := os.Stat(worktreePath)
	return err == nil
}

// GetCurrentBranch returns the current branch name in a worktree.
func (m *WorktreeManager) GetCurrentBranch(ctx context.Context, id TrailID) (string, error) {
	worktreePath := m.WorktreePath(id)

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetHeadCommit returns the HEAD commit hash in a worktree.
func (m *WorktreeManager) GetHeadCommit(ctx context.Context, id TrailID) (string, error) {
	worktreePath := m.WorktreePath(id)

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD commit: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// branchExists checks if a branch exists in the repository.
//
//nolint:unparam // error return reserved for future use (e.g., remote branch checking)
func (m *WorktreeManager) branchExists(ctx context.Context, branch string) (bool, error) {
	//nolint:gosec // branch name is from trail config
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "refs/heads/"+branch)
	cmd.Dir = m.repoRoot
	if err := cmd.Run(); err != nil {
		// Branch doesn't exist - this is expected, not an error
		return false, nil //nolint:nilerr // git exits non-zero when branch doesn't exist
	}
	return true, nil
}

// getDefaultBranch returns the repository's default branch name.
func (m *WorktreeManager) getDefaultBranch(ctx context.Context) (string, error) {
	// Try to get the default branch from origin
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = m.repoRoot
	if output, err := cmd.Output(); err == nil {
		ref := strings.TrimSpace(string(output))
		// Extract branch name from refs/remotes/origin/main
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// Fall back to checking common default branch names
	for _, branch := range []string{"main", "master"} {
		exists, err := m.branchExists(ctx, branch)
		if err != nil {
			continue
		}
		if exists {
			return branch, nil
		}
	}

	return "", errors.New("could not determine default branch")
}

// List returns all trail worktrees.
func (m *WorktreeManager) List() ([]string, error) {
	worktreesDir := filepath.Join(m.repoRoot, WorktreeDir)

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read worktrees directory: %w", err)
	}

	var worktrees []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "trail-") {
			worktrees = append(worktrees, filepath.Join(worktreesDir, entry.Name()))
		}
	}

	return worktrees, nil
}

// EnsureWorktreesIgnored ensures the .worktrees directory is in .gitignore.
func EnsureWorktreesIgnored(repoRoot string) error {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")

	// Read existing .gitignore
	//nolint:gosec // gitignorePath is constructed from validated repoRoot
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	// Check if already ignored
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == WorktreeDir || trimmed == WorktreeDir+"/" {
			return nil // Already ignored
		}
	}

	// Add to .gitignore
	var newContent string
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		newContent = string(content) + "\n" + WorktreeDir + "/\n"
	} else {
		newContent = string(content) + WorktreeDir + "/\n"
	}

	//nolint:gosec // .gitignore needs to be readable
	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("failed to update .gitignore: %w", err)
	}

	return nil
}

// GetRepoRoot returns the repository root, using the paths package.
func GetRepoRoot() (string, error) {
	root, err := paths.RepoRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get repository root: %w", err)
	}
	return root, nil
}
