package insightsdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SessionRow represents a single session for insertion into the cache.
// This is the denormalized view used by the CLI — callers populate it
// from checkpoint.CommittedMetadata.
type SessionRow struct {
	CheckpointID string
	SessionID    string
	SessionIndex int
	Agent        string
	Model        string
	Branch       string
	CreatedAt    time.Time
	InputTokens  int
	CacheTokens  int
	OutputTokens int
	TotalTokens  int
	APICallCount int
	DurationMs   int64
	TurnCount    int
	Intent       string
	Outcome      string
	AgentPct     float64
	// Score fields (may be zero if not yet computed)
	OverallScore   float64
	ScoreTokenEff  float64
	ScoreFirstPass float64
	ScoreFriction  float64
	ScoreFocus     float64
	// Denormalized arrays
	FilesTouched []string
	Friction     []string
	Learnings    []LearningRow
}

// LearningRow represents a single learning entry within a session.
type LearningRow struct {
	Scope   string // "repo", "workflow", "code"
	Finding string
	Path    string // only meaningful when Scope is "code"
}

// GetBranchTip returns the stored branch tip hash from cache_meta,
// or an empty string if it has not been set yet.
func (idb *InsightsDB) GetBranchTip(ctx context.Context) (string, error) {
	var tip string
	err := idb.db.QueryRowContext(ctx,
		"SELECT value FROM cache_meta WHERE key = ?",
		"branch_tip",
	).Scan(&tip)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get branch tip: %w", err)
	}
	return tip, nil
}

// SetBranchTip stores the branch tip hash in cache_meta.
// Overwrites any previously stored value.
func (idb *InsightsDB) SetBranchTip(ctx context.Context, tip string) error {
	_, err := idb.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO cache_meta (key, value) VALUES (?, ?)",
		"branch_tip",
		tip,
	)
	if err != nil {
		return fmt.Errorf("set branch tip: %w", err)
	}
	return nil
}

// HasCheckpoint returns true if any session for the given checkpoint ID
// is already present in the sessions table.
func (idb *InsightsDB) HasCheckpoint(ctx context.Context, checkpointID string) (bool, error) {
	var count int
	err := idb.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE checkpoint_id = ?",
		checkpointID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check checkpoint existence: %w", err)
	}
	return count > 0, nil
}

// InsertSession inserts a session and all its denormalized data into the cache.
// The insert is performed inside a single transaction so the cache remains
// consistent even if the caller is interrupted mid-insert.
func (idb *InsightsDB) InsertSession(ctx context.Context, row SessionRow) error {
	tx, err := idb.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback() //nolint:errcheck // Rollback after failed tx; error is irrelevant
		}
	}()

	if err = insertSessionRow(ctx, tx, row); err != nil {
		return err
	}
	if err = insertFilesTouched(ctx, tx, row); err != nil {
		return err
	}
	if err = insertFriction(ctx, tx, row); err != nil {
		return err
	}
	if err = insertLearnings(ctx, tx, row); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func insertSessionRow(ctx context.Context, tx *sql.Tx, row SessionRow) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (
			checkpoint_id, session_id, session_index,
			agent, model, branch, created_at,
			input_tokens, cache_tokens, output_tokens, total_tokens,
			api_call_count, duration_ms, turn_count,
			intent, outcome, agent_percentage,
			overall_score, score_token_efficiency, score_first_pass,
			score_friction, score_focus
		) VALUES (
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?
		)`,
		row.CheckpointID, row.SessionID, row.SessionIndex,
		nullableString(row.Agent), nullableString(row.Model), nullableString(row.Branch),
		row.CreatedAt.UTC().Format(time.RFC3339),
		row.InputTokens, row.CacheTokens, row.OutputTokens, row.TotalTokens,
		row.APICallCount, row.DurationMs, row.TurnCount,
		nullableString(row.Intent), nullableString(row.Outcome), row.AgentPct,
		nullableFloat(row.OverallScore), nullableFloat(row.ScoreTokenEff),
		nullableFloat(row.ScoreFirstPass), nullableFloat(row.ScoreFriction),
		nullableFloat(row.ScoreFocus),
	)
	if err != nil {
		return fmt.Errorf("insert session row: %w", err)
	}
	return nil
}

func insertFilesTouched(ctx context.Context, tx *sql.Tx, row SessionRow) error {
	for _, f := range row.FilesTouched {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO files_touched (checkpoint_id, session_index, file_path) VALUES (?, ?, ?)",
			row.CheckpointID, row.SessionIndex, f,
		); err != nil {
			return fmt.Errorf("insert files_touched: %w", err)
		}
	}
	return nil
}

func insertFriction(ctx context.Context, tx *sql.Tx, row SessionRow) error {
	for _, f := range row.Friction {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO friction (checkpoint_id, session_index, text) VALUES (?, ?, ?)",
			row.CheckpointID, row.SessionIndex, f,
		); err != nil {
			return fmt.Errorf("insert friction: %w", err)
		}
	}
	return nil
}

func insertLearnings(ctx context.Context, tx *sql.Tx, row SessionRow) error {
	for _, l := range row.Learnings {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO learnings (checkpoint_id, session_index, scope, finding, path) VALUES (?, ?, ?, ?, ?)",
			row.CheckpointID, row.SessionIndex, l.Scope, l.Finding, nullableString(l.Path),
		); err != nil {
			return fmt.Errorf("insert learnings: %w", err)
		}
	}
	return nil
}

// nullableString converts an empty string to a SQL NULL value.
// Non-empty strings are passed through as-is.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullableFloat converts a zero float to a SQL NULL value.
// Non-zero floats are passed through as-is.
func nullableFloat(f float64) interface{} {
	if f == 0 {
		return nil
	}
	return f
}
