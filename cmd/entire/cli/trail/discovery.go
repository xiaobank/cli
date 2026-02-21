package trail

import (
	"context"
	"slices"
	"sort"

	"github.com/go-git/go-git/v5"
)

// Discovery provides methods to discover and filter trails.
type Discovery struct {
	store *Store
	state *StateManager
}

// NewDiscovery creates a new discovery instance.
func NewDiscovery(repo *git.Repository) *Discovery {
	return &Discovery{
		store: NewStore(repo),
		state: NewStateManager(repo),
	}
}

// ListFilter specifies filtering options for listing trails.
type ListFilter struct {
	// States filters to trails matching any of these states.
	// If empty, returns all trails.
	States []TrailState

	// Labels filters to trails having all of these labels.
	Labels []string

	// Assignees filters to trails assigned to any of these users.
	Assignees []string
}

// ListWithState returns all trails with their current execution state.
func (d *Discovery) ListWithState(ctx context.Context, filter *ListFilter) ([]TrailWithState, error) {
	trails, err := d.store.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []TrailWithState

	for _, trail := range trails {
		state, err := d.state.GetState(ctx, trail.ID)
		if err != nil {
			// Skip trails we can't get state for
			continue
		}

		// Apply state filter
		if filter != nil && len(filter.States) > 0 {
			if !containsState(filter.States, state) {
				continue
			}
		}

		// Apply label filter (AND logic - must have all labels)
		if filter != nil && len(filter.Labels) > 0 {
			if !hasAllLabels(trail.Labels, filter.Labels) {
				continue
			}
		}

		// Apply assignee filter (OR logic - any assignee matches)
		if filter != nil && len(filter.Assignees) > 0 {
			if !hasAnyAssignee(trail.Assignees, filter.Assignees) {
				continue
			}
		}

		result = append(result, TrailWithState{
			Trail: *trail,
			State: state,
		})
	}

	// Sort by created date (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// FindOpen returns trails that are available for execution.
func (d *Discovery) FindOpen(ctx context.Context) ([]TrailWithState, error) {
	return d.ListWithState(ctx, &ListFilter{
		States: []TrailState{TrailStateOpen},
	})
}

// FindInProgress returns trails that are currently being executed.
func (d *Discovery) FindInProgress(ctx context.Context) ([]TrailWithState, error) {
	return d.ListWithState(ctx, &ListFilter{
		States: []TrailState{TrailStateInProgress},
	})
}

// FindCompleted returns trails that have been successfully completed.
func (d *Discovery) FindCompleted(ctx context.Context) ([]TrailWithState, error) {
	return d.ListWithState(ctx, &ListFilter{
		States: []TrailState{TrailStateCompleted},
	})
}

// FindFailed returns trails that failed during execution.
func (d *Discovery) FindFailed(ctx context.Context) ([]TrailWithState, error) {
	return d.ListWithState(ctx, &ListFilter{
		States: []TrailState{TrailStateFailed},
	})
}

// GetWithState retrieves a single trail with its current state.
func (d *Discovery) GetWithState(ctx context.Context, id TrailID) (*TrailWithState, error) {
	trail, err := d.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if trail == nil {
		return nil, nil //nolint:nilnil // Trail not found
	}

	state, err := d.state.GetState(ctx, trail.ID)
	if err != nil {
		return nil, err
	}

	return &TrailWithState{
		Trail: *trail,
		State: state,
	}, nil
}

// containsState checks if a state is in the list of states.
func containsState(states []TrailState, state TrailState) bool {
	return slices.Contains(states, state)
}

// hasAllLabels checks if the trail has all the required labels.
func hasAllLabels(trailLabels, requiredLabels []string) bool {
	labelSet := make(map[string]bool)
	for _, l := range trailLabels {
		labelSet[l] = true
	}
	for _, l := range requiredLabels {
		if !labelSet[l] {
			return false
		}
	}
	return true
}

// hasAnyAssignee checks if the trail has any of the specified assignees.
func hasAnyAssignee(trailAssignees, filterAssignees []string) bool {
	assigneeSet := make(map[string]bool)
	for _, a := range trailAssignees {
		assigneeSet[a] = true
	}
	for _, a := range filterAssignees {
		if assigneeSet[a] {
			return true
		}
	}
	return false
}
