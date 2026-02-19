package cli

import (
	"io"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/benchutil"
	"github.com/entireio/cli/cmd/entire/cli/paths"
)

// BenchmarkStatusCommand benchmarks the `entire status` command end-to-end.
// This is the top-level entry point for understanding status command latency.
//
// Key I/O operations measured:
//   - git rev-parse --show-toplevel (RepoRoot, cached after first call)
//   - git rev-parse --git-common-dir (NewStateStore, per invocation)
//   - git rev-parse --abbrev-ref HEAD (resolveWorktreeBranch, per unique worktree)
//   - os.ReadFile for settings.json, each session state file
//   - JSON unmarshaling for settings and each session state
//
// The primary scaling dimension is active session count.
func BenchmarkStatusCommand(b *testing.B) {
	b.Run("Short/NoSessions", benchStatus(0, false))
	b.Run("Short/1Session", benchStatus(1, false))
	b.Run("Short/5Sessions", benchStatus(5, false))
	b.Run("Short/10Sessions", benchStatus(10, false))
	b.Run("Short/20Sessions", benchStatus(20, false))
	b.Run("Detailed/NoSessions", benchStatus(0, true))
	b.Run("Detailed/5Sessions", benchStatus(5, true))
}

func benchStatus(sessionCount int, detailed bool) func(*testing.B) {
	return func(b *testing.B) {
		repo := benchutil.NewBenchRepo(b, benchutil.RepoOpts{})

		// Create active session state files in .git/entire-sessions/
		for range sessionCount {
			repo.CreateSessionState(b, benchutil.SessionOpts{})
		}

		// runStatus uses paths.RepoRoot() which requires cwd to be in the repo.
		b.Chdir(repo.Dir)
		paths.ClearRepoRootCache()

		b.ResetTimer()
		for range b.N {
			// Clear cache each iteration to simulate a fresh CLI invocation.
			// In real usage, each `entire status` call starts cold.
			paths.ClearRepoRootCache()

			if err := runStatus(io.Discard, detailed); err != nil {
				b.Fatalf("runStatus: %v", err)
			}
		}
	}
}
