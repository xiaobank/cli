package checkpoint

import (
	"context"

	"github.com/entireio/cli/cmd/entire/cli/agent/types"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint/id"
	"github.com/entireio/cli/cmd/entire/cli/filter"

	"github.com/go-git/go-git/v6/plumbing"
)

// SmudgeSessionContent applies the smudge filter to transcript and prompt
// fields of a SessionContent in place. Safe to call with nil content or pipeline.
func SmudgeSessionContent(content *SessionContent, pipeline *filter.Pipeline) {
	if content == nil || pipeline == nil {
		return
	}
	content.Transcript = pipeline.Smudge(content.Transcript)
	content.Prompts = pipeline.SmudgeString(content.Prompts)
}

// ReadSessionContentForDisplay reads a session's content and applies the smudge
// filter so stored placeholders are replaced with machine-specific paths.
// Use this for user-facing output; use ReadSessionContent for internal operations.
func (s *GitStore) ReadSessionContentForDisplay(ctx context.Context, checkpointID id.CheckpointID, sessionIndex int) (*SessionContent, error) {
	content, err := s.ReadSessionContent(ctx, checkpointID, sessionIndex)
	if err != nil {
		return nil, err
	}
	SmudgeSessionContent(content, filter.FromContext(ctx))
	return content, nil
}

// ReadLatestSessionContentForDisplay is the display variant of ReadLatestSessionContent.
func (s *GitStore) ReadLatestSessionContentForDisplay(ctx context.Context, checkpointID id.CheckpointID) (*SessionContent, error) {
	content, err := s.ReadLatestSessionContent(ctx, checkpointID)
	if err != nil {
		return nil, err
	}
	SmudgeSessionContent(content, filter.FromContext(ctx))
	return content, nil
}

// GetSessionLogForDisplay is the display variant of GetSessionLog.
func (s *GitStore) GetSessionLogForDisplay(ctx context.Context, cpID id.CheckpointID) ([]byte, string, error) {
	transcript, sessionID, err := s.GetSessionLog(ctx, cpID)
	if err != nil {
		return nil, "", err
	}
	pipeline := filter.FromContext(ctx)
	return pipeline.Smudge(transcript), sessionID, nil
}

// LookupSessionLogForDisplay is a convenience function that opens the repository
// and retrieves a smudged session log by checkpoint ID.
func LookupSessionLogForDisplay(ctx context.Context, cpID id.CheckpointID) ([]byte, string, error) {
	transcript, sessionID, err := LookupSessionLog(ctx, cpID)
	if err != nil {
		return nil, "", err
	}
	pipeline := filter.FromContext(ctx)
	return pipeline.Smudge(transcript), sessionID, nil
}

// GetTranscriptFromCommitForDisplay is the display variant of GetTranscriptFromCommit.
func (s *GitStore) GetTranscriptFromCommitForDisplay(ctx context.Context, commitHash plumbing.Hash, metadataDir string, agentType types.AgentType) ([]byte, error) {
	transcript, err := s.GetTranscriptFromCommit(ctx, commitHash, metadataDir, agentType)
	if err != nil {
		return nil, err
	}
	pipeline := filter.FromContext(ctx)
	return pipeline.Smudge(transcript), nil
}
