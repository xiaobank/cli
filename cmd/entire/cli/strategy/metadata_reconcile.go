package strategy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/paths"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ReconcileDisconnectedMetadataBranch detects and repairs disconnected local/remote
// entire/checkpoints/v1 branches. Disconnected means no common ancestor, which
// only happens due to the empty-orphan bug. Diverged (shared ancestor) is normal
// and handled by the push path's tree merge.
//
// Repair strategy: cherry-pick local commits onto remote tip, preserving all data.
// Checkpoint shards use unique paths (<id[:2]>/<id[2:]>/), so cherry-picks always
// apply cleanly.
func ReconcileDisconnectedMetadataBranch(repo *git.Repository) error {
	refName := plumbing.NewBranchReferenceName(paths.MetadataBranchName)

	// Check local branch
	localRef, err := repo.Reference(refName, true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil // No local branch — nothing to reconcile
	}
	if err != nil {
		return fmt.Errorf("failed to check local metadata branch: %w", err)
	}

	// Check remote-tracking branch
	remoteRefName := plumbing.NewRemoteReferenceName("origin", paths.MetadataBranchName)
	remoteRef, err := repo.Reference(remoteRefName, true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil // No remote branch — nothing to reconcile
	}
	if err != nil {
		return fmt.Errorf("failed to check remote metadata branch: %w", err)
	}

	localHash := localRef.Hash()
	remoteHash := remoteRef.Hash()

	// Same hash — nothing to do
	if localHash == remoteHash {
		return nil
	}

	// Check if disconnected using git merge-base
	repoPath, err := getRepoPath(repo)
	if err != nil {
		return err
	}

	disconnected, err := isDisconnected(repoPath, localHash.String(), remoteHash.String())
	if err != nil {
		return fmt.Errorf("failed to check metadata branch ancestry: %w", err)
	}
	if !disconnected {
		// Shared ancestry (diverged or ancestor) — not our problem
		return nil
	}

	// Disconnected — cherry-pick local commits onto remote tip
	fmt.Fprintln(os.Stderr, "[entire] Detected disconnected session metadata (local and remote share no common ancestor)")

	// Collect local commits oldest-first
	localCommits, err := collectCommitChain(repo, localHash)
	if err != nil {
		return fmt.Errorf("failed to collect local commits: %w", err)
	}

	// Filter out empty-tree commits (the orphan bug commit)
	var dataCommits []*object.Commit
	for _, c := range localCommits {
		tree, treeErr := c.Tree()
		if treeErr != nil {
			return fmt.Errorf("failed to read tree for commit %s: %w", c.Hash.String()[:7], treeErr)
		}
		if len(tree.Entries) > 0 {
			dataCommits = append(dataCommits, c)
		}
	}

	if len(dataCommits) == 0 {
		// Local only had empty orphan — just point to remote
		ref := plumbing.NewHashReference(refName, remoteHash)
		if err := repo.Storer.SetReference(ref); err != nil {
			return fmt.Errorf("failed to reset metadata branch to remote: %w", err)
		}
		fmt.Fprintln(os.Stderr, "[entire] Done — local had no checkpoint data, reset to remote")
		return nil
	}

	fmt.Fprintf(os.Stderr, "[entire] Cherry-picking %d local checkpoint(s) onto remote...\n", len(dataCommits))

	newTip, err := cherryPickOnto(repo, remoteHash, dataCommits)
	if err != nil {
		return fmt.Errorf("failed to cherry-pick local commits onto remote: %w", err)
	}

	// Update local branch ref
	ref := plumbing.NewHashReference(refName, newTip)
	if err := repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("failed to update metadata branch: %w", err)
	}

	fmt.Fprintln(os.Stderr, "[entire] Done — all local and remote checkpoints preserved")
	return nil
}

// isDisconnected checks if two commits have no common ancestor using git merge-base.
// Returns (true, nil) if disconnected, (false, nil) if they share ancestry,
// or (false, error) if git merge-base failed for another reason.
//
// git merge-base exit codes:
//   - 0: common ancestor found (shared ancestry)
//   - 1: no common ancestor (disconnected)
//   - 128+: error (corrupt repo, invalid hash, etc.)
func isDisconnected(repoPath, hashA, hashB string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "merge-base", hashA, hashB)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true, nil // No common ancestor — disconnected
		}
		return false, fmt.Errorf("git merge-base failed: %w", err)
	}
	return false, nil // Shared ancestry
}

// collectCommitChain walks from tip to root following first parent, returns oldest-first.
func collectCommitChain(repo *git.Repository, tip plumbing.Hash) ([]*object.Commit, error) {
	var chain []*object.Commit
	current := tip

	reachedRoot := false
	for range MaxCommitTraversalDepth {
		commit, err := repo.CommitObject(current)
		if err != nil {
			return nil, fmt.Errorf("failed to get commit %s: %w", current, err)
		}
		chain = append(chain, commit)

		if len(commit.ParentHashes) == 0 {
			reachedRoot = true
			break
		}
		current = commit.ParentHashes[0]
	}

	if !reachedRoot {
		return nil, fmt.Errorf("commit chain exceeded %d commits without reaching root; aborting reconciliation", MaxCommitTraversalDepth)
	}

	// Reverse to oldest-first
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	return chain, nil
}

// cherryPickOnto applies each commit's delta onto base, building a linear chain.
// For each commit, it computes the diff from its parent using DiffTrees, then
// applies that delta onto the current tip's tree using ApplyTreeChanges.
func cherryPickOnto(repo *git.Repository, base plumbing.Hash, commits []*object.Commit) (plumbing.Hash, error) {
	currentTip := base

	for _, commit := range commits {
		// Flatten the commit's tree and its parent's tree to compute the delta
		commitTree, err := commit.Tree()
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("failed to get tree for commit %s: %w", commit.Hash, err)
		}
		commitEntries := make(map[string]object.TreeEntry)
		if err := checkpoint.FlattenTree(repo, commitTree, "", commitEntries); err != nil {
			return plumbing.ZeroHash, fmt.Errorf("failed to flatten commit tree: %w", err)
		}

		parentEntries := make(map[string]object.TreeEntry)
		if len(commit.ParentHashes) > 0 {
			parentCommit, pErr := repo.CommitObject(commit.ParentHashes[0])
			if pErr != nil {
				return plumbing.ZeroHash, fmt.Errorf("failed to get parent commit %s: %w", commit.ParentHashes[0], pErr)
			}
			parentTree, ptErr := parentCommit.Tree()
			if ptErr != nil {
				return plumbing.ZeroHash, fmt.Errorf("failed to get parent tree for commit %s: %w", commit.ParentHashes[0], ptErr)
			}
			if err := checkpoint.FlattenTree(repo, parentTree, "", parentEntries); err != nil {
				return plumbing.ZeroHash, fmt.Errorf("failed to flatten parent tree for commit %s: %w", commit.ParentHashes[0], err)
			}
		}

		// Compute delta and apply surgically to the tip tree
		changes := checkpoint.DiffTrees(parentEntries, commitEntries)
		if len(changes) == 0 {
			continue // Skip no-op commits
		}

		tipCommit, err := repo.CommitObject(currentTip)
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("failed to get tip commit: %w", err)
		}

		mergedTreeHash, err := checkpoint.ApplyTreeChanges(repo, tipCommit.TreeHash, changes)
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("failed to apply tree changes: %w", err)
		}

		newHash, err := createCherryPickCommit(repo, mergedTreeHash, currentTip, commit)
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("failed to create cherry-pick commit: %w", err)
		}

		currentTip = newHash
	}

	return currentTip, nil
}

// createCherryPickCommit creates a new commit on top of parent, preserving the
// original commit's message and author.
func createCherryPickCommit(repo *git.Repository, treeHash, parent plumbing.Hash, original *object.Commit) (plumbing.Hash, error) {
	authorName, authorEmail := GetGitAuthorFromRepo(repo)
	now := time.Now()

	commit := &object.Commit{
		TreeHash:     treeHash,
		ParentHashes: []plumbing.Hash{parent},
		Author:       original.Author,
		Committer: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  now,
		},
		Message: original.Message,
	}

	obj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to encode commit: %w", err)
	}

	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to store commit: %w", err)
	}

	return hash, nil
}

// getRepoPath returns the filesystem path for the repository's worktree.
func getRepoPath(repo *git.Repository) (string, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}
	return wt.Filesystem.Root(), nil
}
