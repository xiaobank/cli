package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkActiveSessionsPushed(t *testing.T) {
	t.Run("sets flag on ACTIVE session with TurnCheckpointIDs", func(t *testing.T) {
		dir := setupGitRepo(t)
		t.Chdir(dir)
		session.ClearGitCommonDirCache()

		ctx := context.Background()

		state := &SessionState{
			SessionID:         "test-push-active-1",
			BaseCommit:        "abc123",
			StartedAt:         time.Now(),
			Phase:             session.PhaseActive,
			TurnCheckpointIDs: []string{"cp-1", "cp-2"},
		}
		require.NoError(t, SaveSessionState(ctx, state))

		markActiveSessionsPushed(ctx, "origin")

		loaded, err := LoadSessionState(ctx, "test-push-active-1")
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Equal(t, "origin", loaded.PushedDuringTurnRemote,
			"PushedDuringTurnRemote should be set to the remote name")
	})

	t.Run("skips IDLE session", func(t *testing.T) {
		dir := setupGitRepo(t)
		t.Chdir(dir)
		session.ClearGitCommonDirCache()

		ctx := context.Background()

		state := &SessionState{
			SessionID:         "test-push-idle-1",
			BaseCommit:        "abc123",
			StartedAt:         time.Now(),
			Phase:             session.PhaseIdle,
			TurnCheckpointIDs: []string{"cp-1"},
		}
		require.NoError(t, SaveSessionState(ctx, state))

		markActiveSessionsPushed(ctx, "origin")

		loaded, err := LoadSessionState(ctx, "test-push-idle-1")
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Empty(t, loaded.PushedDuringTurnRemote,
			"PushedDuringTurnRemote should NOT be set on IDLE sessions")
	})

	t.Run("skips ACTIVE session without TurnCheckpointIDs", func(t *testing.T) {
		dir := setupGitRepo(t)
		t.Chdir(dir)
		session.ClearGitCommonDirCache()

		ctx := context.Background()

		state := &SessionState{
			SessionID:  "test-push-no-cps-1",
			BaseCommit: "abc123",
			StartedAt:  time.Now(),
			Phase:      session.PhaseActive,
			// No TurnCheckpointIDs
		}
		require.NoError(t, SaveSessionState(ctx, state))

		markActiveSessionsPushed(ctx, "origin")

		loaded, err := LoadSessionState(ctx, "test-push-no-cps-1")
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Empty(t, loaded.PushedDuringTurnRemote,
			"PushedDuringTurnRemote should NOT be set when TurnCheckpointIDs is empty")
	})
}
