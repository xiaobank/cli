package trail

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/jsonutil"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// TrailsBranchName is the orphan branch where trail definitions are stored.
const TrailsBranchName = "entire/trails"

// MetadataFileName is the name of the metadata file within each trail directory.
const MetadataFileName = "metadata.json"

// Store provides read/write access to trails stored in the entire/trails branch.
type Store struct {
	repo *git.Repository
}

// NewStore creates a new trail store for the given repository.
func NewStore(repo *git.Repository) *Store {
	return &Store{repo: repo}
}

// EnsureBranch creates the entire/trails orphan branch if it doesn't exist.
func (s *Store) EnsureBranch(ctx context.Context) error {
	_ = ctx // Reserved for future use

	refName := plumbing.NewBranchReferenceName(TrailsBranchName)
	_, err := s.repo.Reference(refName, true)
	if err == nil {
		return nil // Branch exists
	}

	// Create orphan branch with empty tree
	emptyTreeHash, err := checkpoint.BuildTreeFromEntries(s.repo, make(map[string]object.TreeEntry))
	if err != nil {
		return fmt.Errorf("failed to create empty tree: %w", err)
	}

	authorName, authorEmail := checkpoint.GetGitAuthorFromRepo(s.repo)
	commitHash, err := s.createCommit(emptyTreeHash, plumbing.ZeroHash, "Initialize trails branch", authorName, authorEmail)
	if err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	newRef := plumbing.NewHashReference(refName, commitHash)
	if err := s.repo.Storer.SetReference(newRef); err != nil {
		return fmt.Errorf("failed to set branch reference: %w", err)
	}

	return nil
}

// Create writes a new trail to the entire/trails branch.
func (s *Store) Create(ctx context.Context, trail *Trail) error {
	if err := trail.Validate(); err != nil {
		return fmt.Errorf("invalid trail: %w", err)
	}

	// Ensure branch exists
	if err := s.EnsureBranch(ctx); err != nil {
		return err
	}

	// Get current branch tip and flatten tree
	ref, entries, err := s.getBranchEntries()
	if err != nil {
		return err
	}

	// Check if trail already exists
	trailPath := trail.ID.String() + "/" + MetadataFileName
	if _, exists := entries[trailPath]; exists {
		return fmt.Errorf("trail %s already exists", trail.ID)
	}

	// Set timestamps if not set
	now := time.Now().UTC()
	if trail.CreatedAt.IsZero() {
		trail.CreatedAt = now
	}
	if trail.UpdatedAt.IsZero() {
		trail.UpdatedAt = now
	}

	// Marshal trail to JSON
	trailJSON, err := jsonutil.MarshalIndentWithNewline(trail, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal trail: %w", err)
	}

	// Create blob
	blobHash, err := checkpoint.CreateBlobFromContent(s.repo, trailJSON)
	if err != nil {
		return fmt.Errorf("failed to create trail blob: %w", err)
	}

	// Add to entries
	entries[trailPath] = object.TreeEntry{
		Name: trailPath,
		Mode: filemode.Regular,
		Hash: blobHash,
	}

	// Build and commit
	newTreeHash, err := checkpoint.BuildTreeFromEntries(s.repo, entries)
	if err != nil {
		return fmt.Errorf("failed to build tree: %w", err)
	}

	authorName, authorEmail := checkpoint.GetGitAuthorFromRepo(s.repo)
	commitMsg := fmt.Sprintf("Create trail: %s\n\n%s", trail.ID, trail.Title)
	newCommitHash, err := s.createCommit(newTreeHash, ref.Hash(), commitMsg, authorName, authorEmail)
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	refName := plumbing.NewBranchReferenceName(TrailsBranchName)
	newRef := plumbing.NewHashReference(refName, newCommitHash)
	if err := s.repo.Storer.SetReference(newRef); err != nil {
		return fmt.Errorf("failed to update branch reference: %w", err)
	}

	return nil
}

// Get retrieves a trail by ID from the entire/trails branch.
// Returns nil, nil if the trail doesn't exist.
func (s *Store) Get(ctx context.Context, id TrailID) (*Trail, error) {
	_ = ctx // Reserved for future use

	tree, err := s.getBranchTree()
	if err != nil {
		return nil, nil //nolint:nilnil,nilerr // No branch means no trail
	}

	// Try to get the trail directory
	trailTree, err := tree.Tree(id.String())
	if err != nil {
		return nil, nil //nolint:nilnil,nilerr // Trail directory not found
	}

	// Read metadata.json
	metadataFile, err := trailTree.File(MetadataFileName)
	if err != nil {
		return nil, nil //nolint:nilnil,nilerr // metadata.json not found
	}

	content, err := metadataFile.Contents()
	if err != nil {
		return nil, fmt.Errorf("failed to read trail metadata: %w", err)
	}

	var trail Trail
	if err := json.Unmarshal([]byte(content), &trail); err != nil {
		return nil, fmt.Errorf("failed to parse trail metadata: %w", err)
	}

	return &trail, nil
}

// List returns all trails from the entire/trails branch.
//
//nolint:unparam // error return reserved for future use (e.g., pagination, remote fetching)
func (s *Store) List(ctx context.Context) ([]*Trail, error) {
	_ = ctx // Reserved for future use

	tree, err := s.getBranchTree()
	if err != nil {
		return []*Trail{}, nil //nolint:nilerr // No branch means empty list
	}

	var trails []*Trail

	// Each entry at root level should be a trail directory
	for _, entry := range tree.Entries {
		if entry.Mode != filemode.Dir {
			continue
		}

		// Validate that entry name is a valid trail ID
		if err := ValidateTrailID(entry.Name); err != nil {
			continue // Skip invalid directories
		}

		// Get the trail directory
		trailTree, err := s.repo.TreeObject(entry.Hash)
		if err != nil {
			continue
		}

		// Read metadata.json
		metadataFile, err := trailTree.File(MetadataFileName)
		if err != nil {
			continue
		}

		content, err := metadataFile.Contents()
		if err != nil {
			continue
		}

		var trail Trail
		if err := json.Unmarshal([]byte(content), &trail); err != nil {
			continue
		}

		trails = append(trails, &trail)
	}

	return trails, nil
}

// Update modifies an existing trail in the entire/trails branch.
func (s *Store) Update(ctx context.Context, trail *Trail) error {
	_ = ctx // Reserved for future use

	if err := trail.Validate(); err != nil {
		return fmt.Errorf("invalid trail: %w", err)
	}

	// Get current branch tip and flatten tree
	ref, entries, err := s.getBranchEntries()
	if err != nil {
		return err
	}

	// Check if trail exists
	trailPath := trail.ID.String() + "/" + MetadataFileName
	if _, exists := entries[trailPath]; !exists {
		return fmt.Errorf("trail %s not found", trail.ID)
	}

	// Update timestamp
	trail.UpdatedAt = time.Now().UTC()

	// Marshal trail to JSON
	trailJSON, err := jsonutil.MarshalIndentWithNewline(trail, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal trail: %w", err)
	}

	// Create blob
	blobHash, err := checkpoint.CreateBlobFromContent(s.repo, trailJSON)
	if err != nil {
		return fmt.Errorf("failed to create trail blob: %w", err)
	}

	// Update entry
	entries[trailPath] = object.TreeEntry{
		Name: trailPath,
		Mode: filemode.Regular,
		Hash: blobHash,
	}

	// Build and commit
	newTreeHash, err := checkpoint.BuildTreeFromEntries(s.repo, entries)
	if err != nil {
		return fmt.Errorf("failed to build tree: %w", err)
	}

	authorName, authorEmail := checkpoint.GetGitAuthorFromRepo(s.repo)
	commitMsg := fmt.Sprintf("Update trail: %s\n\n%s", trail.ID, trail.Title)
	newCommitHash, err := s.createCommit(newTreeHash, ref.Hash(), commitMsg, authorName, authorEmail)
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	refName := plumbing.NewBranchReferenceName(TrailsBranchName)
	newRef := plumbing.NewHashReference(refName, newCommitHash)
	if err := s.repo.Storer.SetReference(newRef); err != nil {
		return fmt.Errorf("failed to update branch reference: %w", err)
	}

	return nil
}

// Delete removes a trail from the entire/trails branch.
func (s *Store) Delete(ctx context.Context, id TrailID) error {
	_ = ctx // Reserved for future use

	// Get current branch tip and flatten tree
	ref, entries, err := s.getBranchEntries()
	if err != nil {
		return err
	}

	// Find and remove all entries for this trail
	trailPrefix := id.String() + "/"
	found := false
	for key := range entries {
		if len(key) >= len(trailPrefix) && key[:len(trailPrefix)] == trailPrefix {
			delete(entries, key)
			found = true
		}
	}

	if !found {
		return fmt.Errorf("trail %s not found", id)
	}

	// Build and commit
	newTreeHash, err := checkpoint.BuildTreeFromEntries(s.repo, entries)
	if err != nil {
		return fmt.Errorf("failed to build tree: %w", err)
	}

	authorName, authorEmail := checkpoint.GetGitAuthorFromRepo(s.repo)
	commitMsg := "Delete trail: " + id.String()
	newCommitHash, err := s.createCommit(newTreeHash, ref.Hash(), commitMsg, authorName, authorEmail)
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	refName := plumbing.NewBranchReferenceName(TrailsBranchName)
	newRef := plumbing.NewHashReference(refName, newCommitHash)
	if err := s.repo.Storer.SetReference(newRef); err != nil {
		return fmt.Errorf("failed to update branch reference: %w", err)
	}

	return nil
}

// getBranchEntries returns the trails branch reference and flattened tree entries.
func (s *Store) getBranchEntries() (*plumbing.Reference, map[string]object.TreeEntry, error) {
	refName := plumbing.NewBranchReferenceName(TrailsBranchName)
	ref, err := s.repo.Reference(refName, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get trails branch reference: %w", err)
	}

	parentCommit, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get commit object: %w", err)
	}

	baseTree, err := parentCommit.Tree()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get commit tree: %w", err)
	}

	entries := make(map[string]object.TreeEntry)
	if err := checkpoint.FlattenTree(s.repo, baseTree, "", entries); err != nil {
		return nil, nil, fmt.Errorf("failed to flatten tree: %w", err)
	}

	return ref, entries, nil
}

// getBranchTree returns the tree object for the entire/trails branch.
func (s *Store) getBranchTree() (*object.Tree, error) {
	refName := plumbing.NewBranchReferenceName(TrailsBranchName)
	ref, err := s.repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("trails branch not found: %w", err)
	}

	commit, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit object: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit tree: %w", err)
	}

	return tree, nil
}

// createCommit creates a commit with the given tree and parent.
func (s *Store) createCommit(treeHash, parentHash plumbing.Hash, message, authorName, authorEmail string) (plumbing.Hash, error) {
	commit := &object.Commit{
		TreeHash: treeHash,
		Message:  message,
		Author: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
		Committer: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	}

	// Add parent if not zero (not an orphan commit)
	if parentHash != plumbing.ZeroHash {
		commit.ParentHashes = []plumbing.Hash{parentHash}
	}

	obj := s.repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to encode commit: %w", err)
	}

	hash, err := s.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to store commit: %w", err)
	}

	return hash, nil
}
