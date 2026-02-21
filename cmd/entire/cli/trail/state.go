package trail

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// RefPrefix is the prefix for all trail state refs.
const RefPrefix = "refs/entire/trails/"

// State ref suffixes
const (
	RefSuffixClaimed   = "/claimed"
	RefSuffixCompleted = "/completed"
	RefSuffixFailed    = "/failed"
)

// ErrAlreadyClaimed is returned when attempting to claim a trail that is already claimed.
var ErrAlreadyClaimed = errors.New("trail is already claimed")

// ErrNotClaimed is returned when attempting to complete/fail a trail that is not claimed.
var ErrNotClaimed = errors.New("trail is not claimed")

// StateManager manages trail execution state using git refs.
// State is tracked via lightweight refs (no commits for state changes):
//   - refs/entire/trails/<id>/claimed   -> branch HEAD when claimed
//   - refs/entire/trails/<id>/completed -> branch HEAD when done
//   - refs/entire/trails/<id>/failed    -> branch HEAD on error
type StateManager struct {
	repo *git.Repository
}

// NewStateManager creates a new state manager for the given repository.
func NewStateManager(repo *git.Repository) *StateManager {
	return &StateManager{repo: repo}
}

// GetState returns the current execution state of a trail.
//
//nolint:unparam // error return reserved for future use (e.g., remote state checking)
func (m *StateManager) GetState(ctx context.Context, id TrailID) (TrailState, error) {
	_ = ctx // Reserved for future use

	// Check states in priority order: completed > failed > claimed > open
	if m.refExists(id, RefSuffixCompleted) {
		return TrailStateCompleted, nil
	}
	if m.refExists(id, RefSuffixFailed) {
		return TrailStateFailed, nil
	}
	if m.refExists(id, RefSuffixClaimed) {
		return TrailStateInProgress, nil
	}
	return TrailStateOpen, nil
}

// Claim atomically claims a trail for execution.
// Returns ErrAlreadyClaimed if the trail is already claimed.
// The ref points to the current HEAD of the work branch.
func (m *StateManager) Claim(ctx context.Context, id TrailID, branchHead plumbing.Hash) error {
	_ = ctx // Reserved for future use

	refName := m.refName(id, RefSuffixClaimed)

	// Check if already claimed
	if _, err := m.repo.Reference(refName, true); err == nil {
		return ErrAlreadyClaimed
	}

	// Create the claimed ref
	ref := plumbing.NewHashReference(refName, branchHead)
	if err := m.repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("failed to create claimed ref: %w", err)
	}

	return nil
}

// Complete marks a trail as successfully completed.
// The ref points to the final HEAD of the work branch.
func (m *StateManager) Complete(ctx context.Context, id TrailID, branchHead plumbing.Hash) error {
	_ = ctx // Reserved for future use

	// Verify trail was claimed
	claimedRef := m.refName(id, RefSuffixClaimed)
	if _, err := m.repo.Reference(claimedRef, true); err != nil {
		return ErrNotClaimed
	}

	// Create completed ref
	completedRef := plumbing.NewHashReference(m.refName(id, RefSuffixCompleted), branchHead)
	if err := m.repo.Storer.SetReference(completedRef); err != nil {
		return fmt.Errorf("failed to create completed ref: %w", err)
	}

	// Remove claimed ref
	if err := m.repo.Storer.RemoveReference(claimedRef); err != nil {
		// Non-fatal: completed ref was created successfully
		return nil //nolint:nilerr // intentional - completed ref was already created
	}

	return nil
}

// Fail marks a trail as failed.
// The ref points to the HEAD at time of failure.
func (m *StateManager) Fail(ctx context.Context, id TrailID, branchHead plumbing.Hash) error {
	_ = ctx // Reserved for future use

	// Verify trail was claimed
	claimedRef := m.refName(id, RefSuffixClaimed)
	if _, err := m.repo.Reference(claimedRef, true); err != nil {
		return ErrNotClaimed
	}

	// Create failed ref
	failedRef := plumbing.NewHashReference(m.refName(id, RefSuffixFailed), branchHead)
	if err := m.repo.Storer.SetReference(failedRef); err != nil {
		return fmt.Errorf("failed to create failed ref: %w", err)
	}

	// Remove claimed ref
	if err := m.repo.Storer.RemoveReference(claimedRef); err != nil {
		// Non-fatal: failed ref was created successfully
		return nil //nolint:nilerr // intentional - failed ref was already created
	}

	return nil
}

// Release releases a claimed trail without marking it completed or failed.
// This allows another runner to pick up the trail.
func (m *StateManager) Release(ctx context.Context, id TrailID) error {
	_ = ctx // Reserved for future use

	claimedRef := m.refName(id, RefSuffixClaimed)
	if _, err := m.repo.Reference(claimedRef, true); err != nil {
		return ErrNotClaimed
	}

	if err := m.repo.Storer.RemoveReference(claimedRef); err != nil {
		return fmt.Errorf("failed to remove claimed ref: %w", err)
	}

	return nil
}

// Reset removes all state refs for a trail, returning it to open state.
// This is useful for retrying a failed trail.
func (m *StateManager) Reset(ctx context.Context, id TrailID) error {
	_ = ctx // Reserved for future use

	// Remove all state refs
	for _, suffix := range []string{RefSuffixClaimed, RefSuffixCompleted, RefSuffixFailed} {
		refName := m.refName(id, suffix)
		if _, err := m.repo.Reference(refName, true); err == nil {
			if removeErr := m.repo.Storer.RemoveReference(refName); removeErr != nil {
				return fmt.Errorf("failed to remove ref %s: %w", refName, removeErr)
			}
		}
	}

	return nil
}

// GetClaimedRef returns the commit hash that the claimed ref points to.
// Returns plumbing.ZeroHash if not claimed.
//
//nolint:unparam // error return reserved for future use (e.g., remote state checking)
func (m *StateManager) GetClaimedRef(ctx context.Context, id TrailID) (plumbing.Hash, error) {
	_ = ctx // Reserved for future use

	ref, err := m.repo.Reference(m.refName(id, RefSuffixClaimed), true)
	if err != nil {
		return plumbing.ZeroHash, nil //nolint:nilerr // No ref means not claimed
	}
	return ref.Hash(), nil
}

// GetCompletedRef returns the commit hash that the completed ref points to.
// Returns plumbing.ZeroHash if not completed.
//
//nolint:unparam // error return reserved for future use (e.g., remote state checking)
func (m *StateManager) GetCompletedRef(ctx context.Context, id TrailID) (plumbing.Hash, error) {
	_ = ctx // Reserved for future use

	ref, err := m.repo.Reference(m.refName(id, RefSuffixCompleted), true)
	if err != nil {
		return plumbing.ZeroHash, nil //nolint:nilerr // No ref means not completed
	}
	return ref.Hash(), nil
}

// GetFailedRef returns the commit hash that the failed ref points to.
// Returns plumbing.ZeroHash if not failed.
//
//nolint:unparam // error return reserved for future use (e.g., remote state checking)
func (m *StateManager) GetFailedRef(ctx context.Context, id TrailID) (plumbing.Hash, error) {
	_ = ctx // Reserved for future use

	ref, err := m.repo.Reference(m.refName(id, RefSuffixFailed), true)
	if err != nil {
		return plumbing.ZeroHash, nil //nolint:nilerr // No ref means not failed
	}
	return ref.Hash(), nil
}

// refName returns the full reference name for a trail state.
func (m *StateManager) refName(id TrailID, suffix string) plumbing.ReferenceName {
	return plumbing.ReferenceName(RefPrefix + id.String() + suffix)
}

// refExists checks if a ref exists for the given trail and suffix.
func (m *StateManager) refExists(id TrailID, suffix string) bool {
	_, err := m.repo.Reference(m.refName(id, suffix), true)
	return err == nil
}
