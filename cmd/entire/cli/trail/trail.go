// Package trail provides a git-native task queue for autonomous agent execution.
//
// Trails are task definitions stored in the entire/trails orphan branch.
// Execution state is tracked via lightweight git refs (no commits for state changes).
// The Trail Runner discovers open trails, claims them atomically, creates worktrees,
// and runs agents to complete the work.
package trail

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// TrailID is a 12-character hex identifier for trails.
// This follows the same pattern as CheckpointID for consistency.
//
//nolint:recvcheck,revive // UnmarshalJSON requires pointer receiver; name follows CheckpointID pattern
type TrailID string

// EmptyTrailID represents an unset or invalid trail ID.
const EmptyTrailID TrailID = ""

// trailIDPattern is the regex pattern for a valid trail ID: exactly 12 lowercase hex characters.
const trailIDPattern = `[0-9a-f]{12}`

// trailIDRegex validates the format: exactly 12 lowercase hex characters.
var trailIDRegex = regexp.MustCompile(`^` + trailIDPattern + `$`)

// NewTrailID creates a TrailID from a string, validating its format.
// Returns an error if the string is not a valid 12-character hex ID.
func NewTrailID(s string) (TrailID, error) {
	if err := ValidateTrailID(s); err != nil {
		return EmptyTrailID, err
	}
	return TrailID(s), nil
}

// MustTrailID creates a TrailID from a string, panicking if invalid.
// Use only when the ID is known to be valid (e.g., from trusted sources).
func MustTrailID(s string) TrailID {
	id, err := NewTrailID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// GenerateTrailID creates a new random 12-character hex trail ID.
func GenerateTrailID() (TrailID, error) {
	bytes := make([]byte, 6) // 6 bytes = 12 hex chars
	if _, err := rand.Read(bytes); err != nil {
		return EmptyTrailID, fmt.Errorf("failed to generate random trail ID: %w", err)
	}
	return TrailID(hex.EncodeToString(bytes)), nil
}

// ValidateTrailID checks if a string is a valid trail ID format.
// Returns an error if invalid, nil if valid.
func ValidateTrailID(s string) error {
	if !trailIDRegex.MatchString(s) {
		return fmt.Errorf("invalid trail ID %q: must be 12 lowercase hex characters", s)
	}
	return nil
}

// String returns the trail ID as a string.
func (id TrailID) String() string {
	return string(id)
}

// IsEmpty returns true if the trail ID is empty or unset.
func (id TrailID) IsEmpty() bool {
	return id == EmptyTrailID
}

// Short returns the first 8 characters of the trail ID for display.
func (id TrailID) Short() string {
	if len(id) >= 8 {
		return string(id[:8])
	}
	return string(id)
}

// MarshalJSON implements json.Marshaler.
func (id TrailID) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(string(id))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trail ID: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements json.Unmarshaler with validation.
// Returns an error if the JSON string is not a valid 12-character hex ID.
// Empty strings are allowed and result in EmptyTrailID.
func (id *TrailID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("failed to unmarshal trail ID: %w", err)
	}
	// Allow empty strings (represents unset trail ID)
	if s == "" {
		*id = EmptyTrailID
		return nil
	}
	if err := ValidateTrailID(s); err != nil {
		return err
	}
	*id = TrailID(s)
	return nil
}

// TrailState represents the execution state of a trail.
//
//nolint:revive // TrailState name is intentional for clarity; matches TrailID pattern
type TrailState string

const (
	// TrailStateOpen indicates the trail is available for execution.
	// No state refs exist for this trail.
	TrailStateOpen TrailState = "open"

	// TrailStateInProgress indicates the trail is being executed.
	// The claimed ref exists.
	TrailStateInProgress TrailState = "in_progress"

	// TrailStateCompleted indicates the trail finished successfully.
	// The completed ref exists.
	TrailStateCompleted TrailState = "completed"

	// TrailStateFailed indicates the trail execution failed.
	// The failed ref exists.
	TrailStateFailed TrailState = "failed"
)

// Trail represents a task definition stored in the entire/trails branch.
type Trail struct {
	// ID is the unique identifier for this trail.
	ID TrailID `json:"id"`

	// Title is a brief description of the task.
	Title string `json:"title"`

	// Description is the full task description/prompt for the agent.
	Description string `json:"description"`

	// Branch is the name of the branch to create for this trail's work.
	// If empty, defaults to "trail/<id>".
	Branch string `json:"branch"`

	// BaseBranch is the branch to create the work branch from.
	// If empty, defaults to the repository's default branch.
	BaseBranch string `json:"base_branch"`

	// AuthorID identifies who created this trail.
	AuthorID string `json:"author_id,omitempty"`

	// Assignees are optional user IDs that should work on this trail.
	Assignees []string `json:"assignees,omitempty"`

	// Labels are optional tags for categorization.
	Labels []string `json:"labels,omitempty"`

	// CreatedAt is when the trail was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the trail was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate checks if the trail has all required fields.
func (t *Trail) Validate() error {
	if t.ID.IsEmpty() {
		return errors.New("trail ID is required")
	}
	if t.Title == "" {
		return errors.New("trail title is required")
	}
	if t.Description == "" {
		return errors.New("trail description is required")
	}
	return nil
}

// GetBranch returns the branch name for this trail.
// Returns the configured Branch, or a default "trail/<id>" if not set.
func (t *Trail) GetBranch() string {
	if t.Branch != "" {
		return t.Branch
	}
	return "trail/" + t.ID.String()
}

// TrailWithState combines a trail with its current execution state.
//
//nolint:revive // TrailWithState name is intentional for clarity; matches TrailID/TrailState pattern
type TrailWithState struct {
	Trail

	State TrailState `json:"state"`
}
