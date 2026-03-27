// Package skilldb provides a SQLite database for skill analytics.
// It stores skill metadata, session performance data, friction themes,
// missing instructions, and improvement suggestions.
package skilldb

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // SQLite driver
)

// SkillDB wraps a SQLite database for skill analytics.
type SkillDB struct {
	db *sql.DB
}

// Open opens (or creates) the skill analytics database at the given path.
// It sets WAL mode and busy timeout for safe concurrent access,
// then runs migrations to ensure all tables exist.
func Open(dbPath string) (*SkillDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open skill analytics database: %w", err)
	}

	if err = applyPragmas(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	sdb := &SkillDB{db: db}
	if err = sdb.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return sdb, nil
}

// applyPragmas sets performance and safety pragmas on the database.
func applyPragmas(db *sql.DB) error {
	ctx := context.Background()
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("apply pragma %q: %w", pragma, err)
		}
	}
	return nil
}

// Close closes the underlying database connection.
func (sdb *SkillDB) Close() error {
	if err := sdb.db.Close(); err != nil {
		return fmt.Errorf("close skill analytics database: %w", err)
	}
	return nil
}

// migrate creates all tables if they do not already exist.
// It is safe to call multiple times (idempotent).
func (sdb *SkillDB) migrate() error {
	ctx := context.Background()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS skills (
			name            TEXT NOT NULL,
			source_agent    TEXT NOT NULL,
			path            TEXT NOT NULL,
			kind            TEXT NOT NULL,
			discovered_at   TEXT NOT NULL,
			last_seen_at    TEXT NOT NULL,
			PRIMARY KEY (name, source_agent)
		)`,
		`CREATE TABLE IF NOT EXISTS skill_sessions (
			skill_name      TEXT NOT NULL,
			source_agent    TEXT NOT NULL,
			checkpoint_id   TEXT NOT NULL,
			session_index   INTEGER NOT NULL,
			session_id      TEXT,
			agent           TEXT,
			model           TEXT,
			branch          TEXT,
			created_at      TEXT NOT NULL,
			total_tokens    INTEGER DEFAULT 0,
			turn_count      INTEGER DEFAULT 0,
			overall_score   REAL,
			friction_count  INTEGER DEFAULT 0,
			outcome         TEXT,
			PRIMARY KEY (skill_name, source_agent, checkpoint_id, session_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_sessions_created ON skill_sessions(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_skill_sessions_agent ON skill_sessions(agent)`,
		`CREATE TABLE IF NOT EXISTS skill_friction (
			skill_name      TEXT NOT NULL,
			source_agent    TEXT NOT NULL,
			checkpoint_id   TEXT NOT NULL,
			session_index   INTEGER NOT NULL,
			text            TEXT NOT NULL,
			category        TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS skill_missing_instructions (
			skill_name      TEXT NOT NULL,
			source_agent    TEXT NOT NULL,
			checkpoint_id   TEXT NOT NULL,
			session_index   INTEGER NOT NULL,
			instruction     TEXT NOT NULL,
			evidence        TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS skill_improvements (
			id              TEXT PRIMARY KEY,
			skill_name      TEXT NOT NULL,
			source_agent    TEXT NOT NULL,
			title           TEXT NOT NULL,
			description     TEXT,
			diff            TEXT,
			priority        TEXT DEFAULT 'medium',
			status          TEXT DEFAULT 'pending',
			created_at      TEXT NOT NULL,
			applied_at      TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS cache_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := sdb.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("execute migration statement: %w", err)
		}
	}
	return nil
}

// ListTables returns the names of all user tables in the database.
// This is used in tests to verify migrations ran correctly.
func (sdb *SkillDB) ListTables(ctx context.Context) ([]string, error) {
	rows, err := sdb.db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' ORDER BY name",
	)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables: %w", err)
	}
	return tables, nil
}
