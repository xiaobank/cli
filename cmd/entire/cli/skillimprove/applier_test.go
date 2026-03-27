package skillimprove_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/skillimprove"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestApplyDiff_SimpleAdd(t *testing.T) {
	t.Parallel()

	path := writeTestFile(t, "line1\nline2\nline3\n")

	diff := `--- a/file.md
+++ b/file.md
@@ -1,3 +1,4 @@
 line1
 line2
+newline
 line3`

	err := skillimprove.ApplyDiff(path, diff)
	require.NoError(t, err)

	got := readTestFile(t, path)
	assert.Equal(t, "line1\nline2\nnewline\nline3\n", got)
}

func TestApplyDiff_SimpleRemove(t *testing.T) {
	t.Parallel()

	path := writeTestFile(t, "line1\nline2\nline3\n")

	diff := `--- a/file.md
+++ b/file.md
@@ -1,3 +1,2 @@
 line1
-line2
 line3`

	err := skillimprove.ApplyDiff(path, diff)
	require.NoError(t, err)

	got := readTestFile(t, path)
	assert.Equal(t, "line1\nline3\n", got)
}

func TestApplyDiff_Replace(t *testing.T) {
	t.Parallel()

	path := writeTestFile(t, "line1\nline2\nline3\n")

	diff := `--- a/file.md
+++ b/file.md
@@ -1,3 +1,3 @@
 line1
-line2
+replaced
 line3`

	err := skillimprove.ApplyDiff(path, diff)
	require.NoError(t, err)

	got := readTestFile(t, path)
	assert.Equal(t, "line1\nreplaced\nline3\n", got)
}

func TestApplyDiff_ContextMismatch(t *testing.T) {
	t.Parallel()

	path := writeTestFile(t, "line1\nline2\nline3\n")

	diff := `--- a/file.md
+++ b/file.md
@@ -1,3 +1,4 @@
 line1
 wrong_context
+newline
 line3`

	err := skillimprove.ApplyDiff(path, diff)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "diff context mismatch")
	assert.Contains(t, err.Error(), "wrong_context")
	assert.Contains(t, err.Error(), "line2")
}

func TestApplyDiff_MultipleHunks(t *testing.T) {
	t.Parallel()

	path := writeTestFile(t, "aaa\nbbb\nccc\nddd\neee\nfff\n")

	diff := `--- a/file.md
+++ b/file.md
@@ -1,3 +1,4 @@
 aaa
+inserted1
 bbb
 ccc
@@ -5,2 +6,3 @@
 eee
+inserted2
 fff`

	err := skillimprove.ApplyDiff(path, diff)
	require.NoError(t, err)

	got := readTestFile(t, path)
	assert.Equal(t, "aaa\ninserted1\nbbb\nccc\nddd\neee\ninserted2\nfff\n", got)
}

func TestApplyDiff_EmptyDiff(t *testing.T) {
	t.Parallel()

	path := writeTestFile(t, "line1\nline2\n")

	err := skillimprove.ApplyDiff(path, "")
	require.NoError(t, err)

	got := readTestFile(t, path)
	assert.Equal(t, "line1\nline2\n", got)
}

func TestApplyDiff_FileNotFound(t *testing.T) {
	t.Parallel()

	err := skillimprove.ApplyDiff("/nonexistent/path/file.md", "@@ -1,1 +1,2 @@\n line1\n+line2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading file")
}

func TestParseHunks_SingleHunk(t *testing.T) {
	t.Parallel()

	diff := `--- a/file.md
+++ b/file.md
@@ -1,3 +1,4 @@
 line1
 line2
+newline
 line3`

	hunks, err := skillimprove.ParseHunks(diff)
	require.NoError(t, err)
	require.Len(t, hunks, 1)

	h := hunks[0]
	assert.Equal(t, 1, h.OldStart)
	assert.Equal(t, 3, h.OldCount)
	assert.Equal(t, 1, h.NewStart)
	assert.Equal(t, 4, h.NewCount)
	assert.Len(t, h.Lines, 4)

	assert.Equal(t, ' ', h.Lines[0].Kind)
	assert.Equal(t, "line1", h.Lines[0].Text)

	assert.Equal(t, '+', h.Lines[2].Kind)
	assert.Equal(t, "newline", h.Lines[2].Text)
}

func TestParseHunks_MultipleHunks(t *testing.T) {
	t.Parallel()

	diff := `@@ -1,2 +1,3 @@
 first
+added
 second
@@ -5,2 +6,2 @@
-old
+new
 last`

	hunks, err := skillimprove.ParseHunks(diff)
	require.NoError(t, err)
	require.Len(t, hunks, 2)

	assert.Equal(t, 1, hunks[0].OldStart)
	assert.Equal(t, 2, hunks[0].OldCount)
	assert.Equal(t, 1, hunks[0].NewStart)
	assert.Equal(t, 3, hunks[0].NewCount)

	assert.Equal(t, 5, hunks[1].OldStart)
	assert.Equal(t, 2, hunks[1].OldCount)
	assert.Equal(t, 6, hunks[1].NewStart)
	assert.Equal(t, 2, hunks[1].NewCount)
}

func TestParseHunks_InvalidHeader(t *testing.T) {
	t.Parallel()

	_, err := skillimprove.ParseHunks("@@ invalid @@")
	require.Error(t, err)
}
