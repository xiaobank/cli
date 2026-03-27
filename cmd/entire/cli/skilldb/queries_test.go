package skilldb_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/entireio/cli/cmd/entire/cli/skilldb"
)

func openTestDB(t *testing.T) *skilldb.SkillDB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "skills.db")
	db, err := skilldb.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestUpsertSkill_InsertAndUpdate(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	// Insert a new skill.
	skill := skilldb.SkillRow{
		Name:         "go-linting",
		SourceAgent:  "claude-code",
		Path:         ".codex/skills/go-linting/SKILL.md",
		Kind:         "project",
		DiscoveredAt: now,
		LastSeenAt:   now,
	}
	require.NoError(t, db.UpsertSkill(ctx, skill))

	skills, err := db.ListSkills(ctx)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "go-linting", skills[0].Name)
	assert.Equal(t, ".codex/skills/go-linting/SKILL.md", skills[0].Path)

	// Upsert with different path and later last_seen_at.
	later := now.Add(time.Hour)
	updated := skilldb.SkillRow{
		Name:         "go-linting",
		SourceAgent:  "claude-code",
		Path:         ".codex/skills/go-linting/v2/SKILL.md",
		Kind:         "project",
		DiscoveredAt: later, // Should NOT overwrite original discovered_at
		LastSeenAt:   later,
	}
	require.NoError(t, db.UpsertSkill(ctx, updated))

	skills, err = db.ListSkills(ctx)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, ".codex/skills/go-linting/v2/SKILL.md", skills[0].Path, "path should be updated")
	assert.Equal(t, now, skills[0].DiscoveredAt, "discovered_at should be preserved")
	assert.Equal(t, later, skills[0].LastSeenAt, "last_seen_at should be updated")
}

func TestListSkills(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	skills := []skilldb.SkillRow{
		{Name: "alpha-skill", SourceAgent: "claude-code", Path: "/a", Kind: "project", DiscoveredAt: now, LastSeenAt: now},
		{Name: "beta-skill", SourceAgent: "gemini-cli", Path: "/b", Kind: "global", DiscoveredAt: now, LastSeenAt: now},
	}
	for _, s := range skills {
		require.NoError(t, db.UpsertSkill(ctx, s))
	}

	result, err := db.ListSkills(ctx)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "alpha-skill", result[0].Name)
	assert.Equal(t, "beta-skill", result[1].Name)
}

func TestSkillStats_Aggregation(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions := []skilldb.SkillSessionRow{
		{SkillName: "test-skill", SourceAgent: "claude-code", CheckpointID: "chk-1", SessionIndex: 0, CreatedAt: base, TotalTokens: 1000, OverallScore: 0.8, FrictionCount: 2},
		{SkillName: "test-skill", SourceAgent: "claude-code", CheckpointID: "chk-2", SessionIndex: 0, CreatedAt: base.Add(24 * time.Hour), TotalTokens: 2000, OverallScore: 0.6, FrictionCount: 1},
		{SkillName: "test-skill", SourceAgent: "claude-code", CheckpointID: "chk-3", SessionIndex: 0, CreatedAt: base.Add(48 * time.Hour), TotalTokens: 1500, OverallScore: 0.9, FrictionCount: 0},
	}
	for _, s := range sessions {
		require.NoError(t, db.InsertSession(ctx, s))
	}

	stats, err := db.SkillStats(ctx, "test-skill", "claude-code")
	require.NoError(t, err)
	assert.Equal(t, 3, stats.TotalSessions)
	assert.InDelta(t, (0.8+0.6+0.9)/3, stats.AvgScore, 0.01)
	assert.Equal(t, 3, stats.TotalFriction)
	assert.Equal(t, int64(4500), stats.TotalTokens)
	assert.Equal(t, base, stats.FirstUsed)
	assert.Equal(t, base.Add(48*time.Hour), stats.LastUsed)
}

func TestRecentSessions(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := range 5 {
		s := skilldb.SkillSessionRow{
			SkillName:    "test-skill",
			SourceAgent:  "claude-code",
			CheckpointID: "chk-" + string(rune('A'+i)),
			SessionIndex: 0,
			CreatedAt:    base.Add(time.Duration(i) * time.Hour),
			Agent:        "claude-code",
		}
		require.NoError(t, db.InsertSession(ctx, s))
	}

	sessions, err := db.RecentSessions(ctx, "test-skill", "claude-code", 3)
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	// Should be ordered newest first.
	for i := 1; i < len(sessions); i++ {
		assert.True(t, sessions[i-1].CreatedAt.After(sessions[i].CreatedAt),
			"sessions should be ordered newest first")
	}
}

func TestSkillFrictionThemes_Grouping(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	// Insert friction with same text across different checkpoints.
	require.NoError(t, db.InsertFriction(ctx, "test-skill", "claude-code", "chk-1", 0, "lint failed", "tooling"))
	require.NoError(t, db.InsertFriction(ctx, "test-skill", "claude-code", "chk-2", 0, "lint failed", "tooling"))
	require.NoError(t, db.InsertFriction(ctx, "test-skill", "claude-code", "chk-3", 0, "unique issue", ""))

	themes, err := db.SkillFrictionThemes(ctx, "test-skill", "claude-code")
	require.NoError(t, err)
	require.Len(t, themes, 2)

	// "lint failed" should be first (higher count).
	assert.Equal(t, "lint failed", themes[0].Text)
	assert.Equal(t, 2, themes[0].Count)
	assert.Len(t, themes[0].Sessions, 2)

	assert.Equal(t, "unique issue", themes[1].Text)
	assert.Equal(t, 1, themes[1].Count)
}

func TestSkillMissingInstructions_Grouping(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.InsertMissingInstruction(ctx, "test-skill", "claude-code", "chk-1", 0, "run tests first", "user asked twice"))
	require.NoError(t, db.InsertMissingInstruction(ctx, "test-skill", "claude-code", "chk-2", 0, "run tests first", "user reminded again"))

	instructions, err := db.SkillMissingInstructions(ctx, "test-skill", "claude-code")
	require.NoError(t, err)
	require.Len(t, instructions, 1)
	assert.Equal(t, "run tests first", instructions[0].Instruction)
	assert.Equal(t, 2, instructions[0].Count)
	assert.Len(t, instructions[0].Sessions, 2)
}

func TestAgentBreakdown(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	sessions := []skilldb.SkillSessionRow{
		{SkillName: "test-skill", SourceAgent: "claude-code", CheckpointID: "chk-1", SessionIndex: 0, Agent: "claude-code", CreatedAt: now, TotalTokens: 1000, OverallScore: 0.8},
		{SkillName: "test-skill", SourceAgent: "claude-code", CheckpointID: "chk-2", SessionIndex: 0, Agent: "claude-code", CreatedAt: now, TotalTokens: 2000, OverallScore: 0.6},
		{SkillName: "test-skill", SourceAgent: "claude-code", CheckpointID: "chk-3", SessionIndex: 0, Agent: "gemini-cli", CreatedAt: now, TotalTokens: 1500, OverallScore: 0.9},
	}
	for _, s := range sessions {
		require.NoError(t, db.InsertSession(ctx, s))
	}

	breakdown, err := db.AgentBreakdown(ctx, "test-skill", "claude-code")
	require.NoError(t, err)
	require.Len(t, breakdown, 2)

	// claude-code should be first (more sessions).
	assert.Equal(t, "claude-code", breakdown[0].Agent)
	assert.Equal(t, 2, breakdown[0].SessionCount)
	assert.Equal(t, int64(3000), breakdown[0].TotalTokens)

	assert.Equal(t, "gemini-cli", breakdown[1].Agent)
	assert.Equal(t, 1, breakdown[1].SessionCount)
}

func TestInsertAndListImprovements(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	improvements := []skilldb.SkillImprovement{
		{ID: "imp-1", SkillName: "test-skill", SourceAgent: "claude-code", Title: "Add lint check", Priority: "high", Status: "pending", CreatedAt: now},
		{ID: "imp-2", SkillName: "test-skill", SourceAgent: "claude-code", Title: "Fix typo", Priority: "low", Status: "applied", CreatedAt: now, AppliedAt: &now},
	}
	for _, imp := range improvements {
		require.NoError(t, db.InsertImprovement(ctx, imp))
	}

	// List all.
	all, err := db.ListImprovements(ctx, "test-skill", "claude-code", "")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// List pending only.
	pending, err := db.ListImprovements(ctx, "test-skill", "claude-code", "pending")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "imp-1", pending[0].ID)

	// List applied only.
	applied, err := db.ListImprovements(ctx, "test-skill", "claude-code", "applied")
	require.NoError(t, err)
	require.Len(t, applied, 1)
	assert.Equal(t, "imp-2", applied[0].ID)
}

func TestUpdateImprovementStatus(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	imp := skilldb.SkillImprovement{
		ID:          "imp-update",
		SkillName:   "test-skill",
		SourceAgent: "claude-code",
		Title:       "Improve error handling",
		Priority:    "medium",
		Status:      "pending",
		CreatedAt:   now,
	}
	require.NoError(t, db.InsertImprovement(ctx, imp))

	// Update to applied.
	require.NoError(t, db.UpdateImprovementStatus(ctx, "imp-update", "applied"))

	improvements, err := db.ListImprovements(ctx, "test-skill", "claude-code", "applied")
	require.NoError(t, err)
	require.Len(t, improvements, 1)
	assert.Equal(t, "applied", improvements[0].Status)
	assert.NotNil(t, improvements[0].AppliedAt)
}

func TestCacheTip_GetAndSet(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	// Empty initially.
	tip, err := db.GetCacheTip(ctx)
	require.NoError(t, err)
	assert.Empty(t, tip)

	// Set and get.
	require.NoError(t, db.SetCacheTip(ctx, "abc123"))
	tip, err = db.GetCacheTip(ctx)
	require.NoError(t, err)
	assert.Equal(t, "abc123", tip)

	// Overwrite.
	require.NoError(t, db.SetCacheTip(ctx, "def456"))
	tip, err = db.GetCacheTip(ctx)
	require.NoError(t, err)
	assert.Equal(t, "def456", tip)
}

func TestBeginTx_BulkInserts(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tx, err := db.BeginTx(ctx)
	require.NoError(t, err)

	for i := range 3 {
		row := skilldb.SkillSessionRow{
			SkillName:    "bulk-skill",
			SourceAgent:  "claude-code",
			CheckpointID: "chk-bulk-" + string(rune('A'+i)),
			SessionIndex: 0,
			CreatedAt:    now,
		}
		require.NoError(t, db.InsertSessionTx(ctx, tx, row))
		require.NoError(t, db.InsertFrictionTx(ctx, tx, "bulk-skill", "claude-code", row.CheckpointID, 0, "friction text", "category"))
		require.NoError(t, db.InsertMissingInstructionTx(ctx, tx, "bulk-skill", "claude-code", row.CheckpointID, 0, "instruction", "evidence"))
	}

	require.NoError(t, tx.Commit())

	sessions, err := db.RecentSessions(ctx, "bulk-skill", "claude-code", 10)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)

	themes, err := db.SkillFrictionThemes(ctx, "bulk-skill", "claude-code")
	require.NoError(t, err)
	assert.Len(t, themes, 1)
	assert.Equal(t, 3, themes[0].Count)
}
