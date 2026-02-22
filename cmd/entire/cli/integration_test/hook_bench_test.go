//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/benchutil"
)

// BenchmarkHookSessionStart measures the end-to-end latency of the
// "entire hooks claude-code session-start" subprocess, which is what
// Claude Code users experience on every startup.
//
// Each sub-benchmark isolates a single scaling dimension while holding
// everything else at a small baseline.
//
// Run all:
//
//	go test -tags=integration -bench=BenchmarkHookSessionStart -benchtime=5x -run='^$' -timeout=10m ./cmd/entire/cli/integration_test/...
//
// Run one dimension:
//
//	go test -tags=integration -bench=BenchmarkHookSessionStart/Sessions -benchtime=5x -run='^$' ./cmd/entire/cli/integration_test/...
func BenchmarkHookSessionStart(b *testing.B) {
	b.Run("Sessions", benchSessionCount)
	b.Run("Refs", benchRefCount)
	b.Run("RepoFiles", benchRepoFiles)
	b.Run("Commits", benchCommitHistory)
}

// benchSessionCount scales the number of session state files in .git/entire-sessions/.
// Baseline: 10 files, 1 commit, ~2 refs.
func benchSessionCount(b *testing.B) {
	for _, n := range []int{0, 1, 5, 20, 50, 100} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			repo := benchutil.NewBenchRepo(b, benchutil.RepoOpts{
				FileCount:     10,
				FeatureBranch: "feature/bench",
			})
			for range n {
				repo.CreateSessionState(b, benchutil.SessionOpts{
					StepCount:    3,
					FilesTouched: []string{"src/file_000.go", "src/file_001.go"},
				})
			}
			runSessionStartHook(b, repo)
		})
	}
}

// benchRefCount scales the number of git branches (refs).
// Baseline: 5 session files, 10 files, 1 commit.
func benchRefCount(b *testing.B) {
	for _, n := range []int{0, 10, 50, 200, 500} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			repo := benchutil.NewBenchRepo(b, benchutil.RepoOpts{
				FileCount:     10,
				FeatureBranch: "feature/bench",
			})
			for range 5 {
				repo.CreateSessionState(b, benchutil.SessionOpts{
					StepCount:    3,
					FilesTouched: []string{"src/file_000.go"},
				})
			}
			if n > 0 {
				repo.SeedBranches(b, "feature/team-", n)
				repo.PackRefs(b)
			}
			runSessionStartHook(b, repo)
		})
	}
}

// benchRepoFiles scales the number of tracked files in the repository.
// Baseline: 5 session files, 1 commit, ~2 refs.
func benchRepoFiles(b *testing.B) {
	for _, n := range []int{10, 100, 500, 1000} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			repo := benchutil.NewBenchRepo(b, benchutil.RepoOpts{
				FileCount:     n,
				FileSizeLines: 50,
				FeatureBranch: "feature/bench",
			})
			for range 5 {
				repo.CreateSessionState(b, benchutil.SessionOpts{
					StepCount:    3,
					FilesTouched: []string{"src/file_000.go"},
				})
			}
			runSessionStartHook(b, repo)
		})
	}
}

// benchCommitHistory scales the number of commits in the repository.
// Baseline: 5 session files, 10 files, ~2 refs.
func benchCommitHistory(b *testing.B) {
	for _, n := range []int{1, 10, 50, 200} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			repo := benchutil.NewBenchRepo(b, benchutil.RepoOpts{
				FileCount:     10,
				CommitCount:   n,
				FeatureBranch: "feature/bench",
			})
			for range 5 {
				repo.CreateSessionState(b, benchutil.SessionOpts{
					StepCount:    3,
					FilesTouched: []string{"src/file_000.go"},
				})
			}
			runSessionStartHook(b, repo)
		})
	}
}

// runSessionStartHook is the shared benchmark loop that invokes the session-start
// hook as a subprocess and reports latency in ms/op.
func runSessionStartHook(b *testing.B, repo *benchutil.BenchRepo) {
	b.Helper()

	stdinPayload, err := json.Marshal(map[string]string{
		"session_id":      "bench-session",
		"transcript_path": "",
	})
	if err != nil {
		b.Fatalf("marshal stdin: %v", err)
	}

	binary := getTestBinary()
	claudeProjectDir := b.TempDir()

	b.ResetTimer()
	for range b.N {
		start := time.Now()

		cmd := exec.Command(binary, "hooks", "claude-code", "session-start")
		cmd.Dir = repo.Dir
		cmd.Stdin = bytes.NewReader(stdinPayload)
		cmd.Env = append(os.Environ(),
			"ENTIRE_TEST_CLAUDE_PROJECT_DIR="+claudeProjectDir,
		)

		output, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("session-start hook failed: %v\nOutput: %s", err, output)
		}

		b.ReportMetric(float64(time.Since(start).Milliseconds()), "ms/op")
	}
}
