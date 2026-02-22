package trail

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestTrailID_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid 12 hex chars", "a1b2c3d4e5f6", false},
		{"valid all zeros", "000000000000", false},
		{"valid all f's", "ffffffffffff", false},
		{"too short", "a1b2c3d4e5f", true},
		{"too long", "a1b2c3d4e5f6a", true},
		{"uppercase hex", "A1B2C3D4E5F6", true},
		{"non-hex chars", "g1b2c3d4e5f6", true},
		{"empty", "", true},
		{"with dashes", "a1b2-c3d4-e5f6", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewTrailID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTrailID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestTrailID_Generate(t *testing.T) {
	t.Parallel()

	id1, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}

	id2, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}

	if id1 == id2 {
		t.Error("GenerateTrailID() generated duplicate IDs")
	}

	if len(id1) != 12 {
		t.Errorf("GenerateTrailID() length = %d, want 12", len(id1))
	}
}

func TestTrail_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		trail   Trail
		wantErr bool
	}{
		{
			name: "valid trail",
			trail: Trail{
				ID:          "a1b2c3d4e5f6",
				Title:       "Test Trail",
				Description: "Test description",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			trail: Trail{
				Title:       "Test Trail",
				Description: "Test description",
			},
			wantErr: true,
		},
		{
			name: "missing title",
			trail: Trail{
				ID:          "a1b2c3d4e5f6",
				Description: "Test description",
			},
			wantErr: true,
		},
		{
			name: "missing description",
			trail: Trail{
				ID:    "a1b2c3d4e5f6",
				Title: "Test Trail",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.trail.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTrail_GetBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		trail Trail
		want  string
	}{
		{
			name: "custom branch",
			trail: Trail{
				ID:     "a1b2c3d4e5f6",
				Branch: "feature/custom",
			},
			want: "feature/custom",
		},
		{
			name: "default branch",
			trail: Trail{
				ID: "a1b2c3d4e5f6",
			},
			want: "trail/a1b2c3d4e5f6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.trail.GetBranch(); got != tt.want {
				t.Errorf("GetBranch() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Helper to create a test repository
func createTestRepo(t *testing.T) *git.Repository {
	t.Helper()

	tmpDir := t.TempDir()

	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Create initial commit
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create a file and commit
	testFile := tmpDir + "/README.md"
	if err := os.WriteFile(testFile, []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if _, err := worktree.Add("README.md"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}
	if _, err := worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	}); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	return repo
}

func TestStore_CreateAndGet(t *testing.T) {
	t.Parallel()

	repo := createTestRepo(t)
	store := NewStore(repo)
	ctx := context.Background()

	// Create a trail
	id, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}

	trail := &Trail{
		ID:          id,
		Title:       "Test Trail",
		Description: "This is a test trail",
		Labels:      []string{"test", "example"},
	}

	if err := store.Create(ctx, trail); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Get the trail
	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil")
	}

	if got.ID != id {
		t.Errorf("ID = %q, want %q", got.ID, id)
	}
	if got.Title != "Test Trail" {
		t.Errorf("Title = %q, want %q", got.Title, "Test Trail")
	}
	if got.Description != "This is a test trail" {
		t.Errorf("Description = %q, want %q", got.Description, "This is a test trail")
	}
}

func TestStore_List(t *testing.T) {
	t.Parallel()

	repo := createTestRepo(t)
	store := NewStore(repo)
	ctx := context.Background()

	// Initially empty
	trails, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(trails) != 0 {
		t.Errorf("List() returned %d trails, want 0", len(trails))
	}

	// Create two trails
	for i := range 2 {
		id, err := GenerateTrailID()
		if err != nil {
			t.Fatalf("GenerateTrailID() error = %v", err)
		}
		tr := &Trail{
			ID:          id,
			Title:       "Trail " + string(rune('A'+i)),
			Description: "Description",
		}
		if err := store.Create(ctx, tr); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List should return 2
	trails, err = store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(trails) != 2 {
		t.Errorf("List() returned %d trails, want 2", len(trails))
	}
}

func TestStore_Delete(t *testing.T) {
	t.Parallel()

	repo := createTestRepo(t)
	store := NewStore(repo)
	ctx := context.Background()

	// Create a trail
	id, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}
	tr := &Trail{
		ID:          id,
		Title:       "Test Trail",
		Description: "Description",
	}
	if err := store.Create(ctx, tr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete it
	if err := store.Delete(ctx, id); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Should no longer exist
	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Error("Get() returned trail after delete")
	}
}

func TestStateManager_ClaimAndComplete(t *testing.T) {
	t.Parallel()

	repo := createTestRepo(t)
	state := NewStateManager(repo)
	ctx := context.Background()

	id, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}
	head := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	// Initially open
	got, err := state.GetState(ctx, id)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if got != TrailStateOpen {
		t.Errorf("GetState() = %q, want %q", got, TrailStateOpen)
	}

	// Claim
	if err := state.Claim(ctx, id, head); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}

	// Should be in progress
	got, err = state.GetState(ctx, id)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if got != TrailStateInProgress {
		t.Errorf("GetState() = %q, want %q", got, TrailStateInProgress)
	}

	// Claim again should fail
	if err := state.Claim(ctx, id, head); !errors.Is(err, ErrAlreadyClaimed) {
		t.Errorf("Claim() error = %v, want ErrAlreadyClaimed", err)
	}

	// Complete
	if err := state.Complete(ctx, id, head); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	// Should be completed
	got, err = state.GetState(ctx, id)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if got != TrailStateCompleted {
		t.Errorf("GetState() = %q, want %q", got, TrailStateCompleted)
	}
}

func TestStateManager_Fail(t *testing.T) {
	t.Parallel()

	repo := createTestRepo(t)
	state := NewStateManager(repo)
	ctx := context.Background()

	id, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}
	head := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	// Claim
	if err := state.Claim(ctx, id, head); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}

	// Fail
	if err := state.Fail(ctx, id, head); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}

	// Should be failed
	got, err := state.GetState(ctx, id)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if got != TrailStateFailed {
		t.Errorf("GetState() = %q, want %q", got, TrailStateFailed)
	}
}

func TestStateManager_Reset(t *testing.T) {
	t.Parallel()

	repo := createTestRepo(t)
	state := NewStateManager(repo)
	ctx := context.Background()

	id, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}
	head := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	// Claim and complete
	if err := state.Claim(ctx, id, head); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if err := state.Complete(ctx, id, head); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	// Reset
	if err := state.Reset(ctx, id); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	// Should be open again
	got, err := state.GetState(ctx, id)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if got != TrailStateOpen {
		t.Errorf("GetState() = %q, want %q", got, TrailStateOpen)
	}
}

func TestDiscovery_ListWithState(t *testing.T) {
	t.Parallel()

	repo := createTestRepo(t)
	store := NewStore(repo)
	state := NewStateManager(repo)
	discovery := NewDiscovery(repo)
	ctx := context.Background()

	// Create some trails
	id1, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}
	id2, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}
	id3, err := GenerateTrailID()
	if err != nil {
		t.Fatalf("GenerateTrailID() error = %v", err)
	}

	trails := []*Trail{
		{ID: id1, Title: "Trail 1", Description: "Open trail", Labels: []string{"urgent"}},
		{ID: id2, Title: "Trail 2", Description: "In progress trail"},
		{ID: id3, Title: "Trail 3", Description: "Completed trail"},
	}

	for _, tr := range trails {
		if err := store.Create(ctx, tr); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Set states
	head := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := state.Claim(ctx, id2, head); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if err := state.Claim(ctx, id3, head); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if err := state.Complete(ctx, id3, head); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	// List all
	all, err := discovery.ListWithState(ctx, nil)
	if err != nil {
		t.Fatalf("ListWithState() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListWithState(nil) returned %d trails, want 3", len(all))
	}

	// Filter by state
	open, err := discovery.FindOpen(ctx)
	if err != nil {
		t.Fatalf("FindOpen() error = %v", err)
	}
	if len(open) != 1 {
		t.Errorf("FindOpen() returned %d trails, want 1", len(open))
	}

	// Filter by label
	withLabel, err := discovery.ListWithState(ctx, &ListFilter{Labels: []string{"urgent"}})
	if err != nil {
		t.Fatalf("ListWithState(labels) error = %v", err)
	}
	if len(withLabel) != 1 {
		t.Errorf("ListWithState(labels) returned %d trails, want 1", len(withLabel))
	}
}
