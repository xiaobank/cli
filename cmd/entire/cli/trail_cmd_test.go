package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/trail"
)

func TestPrintTrailDetails_MultiBranch(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)
	m := &trail.Metadata{
		TrailID: "a1b2c3d4e5f6",
		Title:   "Add auth system",
		Status:  trail.StatusActive,
		Author:  "alice",
		Intent:  &trail.Intent{Kind: "inline", Value: "Add OAuth2 authentication"},
		Branches: []trail.BranchEntry{
			{ID: "uuid-1", Name: "feature/auth-core", BaseBranch: "main", Status: trail.BranchOpen, AddedAt: now},
			{ID: "uuid-2", Name: "feature/auth-api", BaseBranch: "feature/auth-core", Status: trail.BranchMerged, PR: &trail.PRRef{Number: 42}, AddedAt: now},
			{ID: "uuid-3", Name: "feature/auth-cleanup", BaseBranch: "main", Status: trail.BranchDiscarded, AddedAt: now},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	var buf bytes.Buffer
	printTrailDetails(&buf, m)
	output := buf.String()

	// Check title and basic fields
	if !strings.Contains(output, "Trail: Add auth system") {
		t.Errorf("expected title in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Status:  active") {
		t.Errorf("expected status in output, got:\n%s", output)
	}

	// Check intent
	if !strings.Contains(output, "Intent:") || !strings.Contains(output, "Add OAuth2 authentication") {
		t.Errorf("expected intent in output, got:\n%s", output)
	}

	// Check branches section
	if !strings.Contains(output, "Branches:") {
		t.Errorf("expected Branches: header in output, got:\n%s", output)
	}

	// Check open branch (space marker)
	if !strings.Contains(output, "feature/auth-core -> main [open]") {
		t.Errorf("expected open branch in output, got:\n%s", output)
	}

	// Check merged branch (+ marker) with PR
	if !strings.Contains(output, "+ feature/auth-api") {
		t.Errorf("expected merged branch with + marker, got:\n%s", output)
	}
	if !strings.Contains(output, "PR #42") {
		t.Errorf("expected PR #42 in output, got:\n%s", output)
	}

	// Check discarded branch (x marker)
	if !strings.Contains(output, "x feature/auth-cleanup") {
		t.Errorf("expected discarded branch with x marker, got:\n%s", output)
	}
}

func TestPrintTrailDetails_LegacySingleBranch(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)
	m := &trail.Metadata{
		TrailID:   "a1b2c3d4e5f6",
		Title:     "Legacy trail",
		Status:    trail.StatusDraft,
		Author:    "bob",
		Branch:    "feature/old",
		Base:      "main",
		CreatedAt: now,
		UpdatedAt: now,
	}

	var buf bytes.Buffer
	printTrailDetails(&buf, m)
	output := buf.String()

	// Should show legacy branch/base
	if !strings.Contains(output, "Branch:  feature/old") {
		t.Errorf("expected legacy branch, got:\n%s", output)
	}
	if !strings.Contains(output, "Base:    main") {
		t.Errorf("expected legacy base, got:\n%s", output)
	}

	// Should NOT show Branches: section
	if strings.Contains(output, "Branches:") {
		t.Errorf("should not show Branches: for legacy trail, got:\n%s", output)
	}
}

func TestPrintTrailDetails_NoIntent(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)
	m := &trail.Metadata{
		TrailID:   "a1b2c3d4e5f6",
		Title:     "No intent trail",
		Status:    trail.StatusDraft,
		Author:    "charlie",
		CreatedAt: now,
		UpdatedAt: now,
	}

	var buf bytes.Buffer
	printTrailDetails(&buf, m)
	output := buf.String()

	if strings.Contains(output, "Intent:") {
		t.Errorf("should not show Intent: when nil, got:\n%s", output)
	}
}
