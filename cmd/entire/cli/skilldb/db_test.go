package skilldb_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/entireio/cli/cmd/entire/cli/skilldb"
)

func TestOpen_CreatesDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "skills.db")

	db, err := skilldb.Open(dbPath)
	require.NoError(t, err)
	require.NotNil(t, db)

	t.Cleanup(func() {
		assert.NoError(t, db.Close())
	})
}

func TestOpen_CreatesAllTables(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "skills.db")

	db, err := skilldb.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()

	tables, err := db.ListTables(ctx)
	require.NoError(t, err)

	expected := []string{
		"cache_meta",
		"skill_friction",
		"skill_improvements",
		"skill_missing_instructions",
		"skill_sessions",
		"skills",
	}
	for _, table := range expected {
		assert.Contains(t, tables, table, "expected table %q to exist", table)
	}
}

func TestOpen_Idempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "skills.db")

	db1, err := skilldb.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db1.Close())

	db2, err := skilldb.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db2.Close())
}

func TestClose_CanBeCalledOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "skills.db")

	db, err := skilldb.Open(dbPath)
	require.NoError(t, err)

	err = db.Close()
	assert.NoError(t, err)
}
