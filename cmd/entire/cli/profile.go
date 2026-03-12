package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/agent/types"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint/id"
	"github.com/entireio/cli/cmd/entire/cli/strategy"
	"github.com/entireio/cli/cmd/entire/cli/stringutil"
	"github.com/entireio/cli/cmd/entire/cli/trailers"
	"github.com/entireio/cli/cmd/entire/cli/transcript"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"

	"github.com/spf13/cobra"
)

const profilePromptMaxLength = 100

type timedMessage struct {
	role      string
	prompt    string
	timestamp time.Time
}

type turnDuration struct {
	prompt   string
	duration time.Duration
}

func newProfileCmd() *cobra.Command {
	var sessionFlag string
	var commitFlag string
	var checkpointFlag string

	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Profile turn durations for a checkpoint transcript",
		Long:  "Profile shows time spent per completed user turn in a checkpoint transcript.",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q\nHint: use --checkpoint, --session, or --commit to specify what to profile", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if checkDisabledGuard(cmd.Context(), cmd.OutOrStdout()) {
				return nil
			}
			return runProfile(cmd.Context(), cmd.OutOrStdout(), sessionFlag, commitFlag, checkpointFlag)
		},
	}

	cmd.Flags().StringVar(&sessionFlag, "session", "", "Profile latest checkpoint for a session ID (or prefix)")
	cmd.Flags().StringVar(&commitFlag, "commit", "", "Profile checkpoint associated with a commit (SHA or ref)")
	cmd.Flags().StringVarP(&checkpointFlag, "checkpoint", "c", "", "Profile a specific checkpoint (ID or prefix)")

	return cmd
}

func runProfile(ctx context.Context, w io.Writer, sessionID, commitRef, checkpointID string) error {
	flagCount := 0
	if sessionID != "" {
		flagCount++
	}
	if commitRef != "" {
		flagCount++
	}
	if checkpointID != "" {
		flagCount++
	}

	if flagCount == 0 {
		return errors.New("must specify one of --session, --commit, --checkpoint")
	}
	if flagCount > 1 {
		return errors.New("cannot specify multiple of --session, --commit, --checkpoint")
	}

	if commitRef != "" {
		return runProfileCommit(ctx, w, commitRef)
	}
	if sessionID != "" {
		return runProfileSession(ctx, w, sessionID)
	}

	return runProfileCheckpoint(ctx, w, checkpointID)
}

func runProfileCommit(ctx context.Context, w io.Writer, commitRef string) error {
	repo, err := openRepository(ctx)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	hash, err := repo.ResolveRevision(plumbing.Revision(commitRef))
	if err != nil {
		return fmt.Errorf("commit not found: %s", commitRef)
	}

	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return fmt.Errorf("failed to get commit: %w", err)
	}

	checkpointID, hasCheckpoint := trailers.ParseCheckpoint(commit.Message)
	if !hasCheckpoint {
		return fmt.Errorf("commit %s does not have an Entire-Checkpoint trailer", hash.String()[:7])
	}

	return runProfileCheckpoint(ctx, w, checkpointID.String())
}

func runProfileSession(ctx context.Context, w io.Writer, sessionPrefix string) error {
	repo, err := openRepository(ctx)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	points, err := getBranchCheckpoints(ctx, repo, branchCheckpointsLimit)
	if err != nil {
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	point, found := selectLatestSessionPoint(points, sessionPrefix)
	if !found {
		return fmt.Errorf("session not found: %s", sessionPrefix)
	}

	if !point.CheckpointID.IsEmpty() {
		return runProfileCheckpoint(ctx, w, point.CheckpointID.String())
	}

	return runProfileCheckpoint(ctx, w, point.ID)
}

func selectLatestSessionPoint(points []strategy.RewindPoint, sessionPrefix string) (strategy.RewindPoint, bool) {
	var latest strategy.RewindPoint
	found := false

	for _, p := range points {
		if p.SessionID != sessionPrefix && !strings.HasPrefix(p.SessionID, sessionPrefix) {
			continue
		}
		if !found || p.Date.After(latest.Date) {
			latest = p
			found = true
		}
	}

	return latest, found
}

func runProfileCheckpoint(ctx context.Context, w io.Writer, checkpointPrefix string) error {
	repo, err := openRepository(ctx)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	store := checkpoint.NewGitStore(repo)
	committed, err := store.ListCommitted(ctx)
	if err != nil {
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	var matches []id.CheckpointID
	for _, info := range committed {
		if strings.HasPrefix(info.CheckpointID.String(), checkpointPrefix) {
			matches = append(matches, info.CheckpointID)
		}
	}

	var checkpointID id.CheckpointID
	switch len(matches) {
	case 0:
		found, tempErr := profileTemporaryCheckpoint(ctx, w, repo, store, checkpointPrefix)
		if found {
			return nil
		}
		if tempErr != nil {
			return tempErr
		}
		return fmt.Errorf("checkpoint not found: %s", checkpointPrefix)
	case 1:
		checkpointID = matches[0]
	default:
		examples := make([]string, 0, 5)
		for i := 0; i < len(matches) && i < 5; i++ {
			examples = append(examples, matches[i].String())
		}
		return fmt.Errorf("ambiguous checkpoint prefix %q matches %d checkpoints: %s", checkpointPrefix, len(matches), strings.Join(examples, ", "))
	}

	content, err := store.ReadLatestSessionContent(ctx, checkpointID)
	if err != nil {
		return fmt.Errorf("failed to read checkpoint content: %w", err)
	}
	if len(content.Transcript) == 0 {
		return fmt.Errorf("checkpoint %s has no transcript", checkpointID)
	}

	turns, err := profileTurns(content.Transcript, content.Metadata.Agent)
	if err != nil {
		return err
	}
	if len(turns) == 0 {
		return fmt.Errorf("checkpoint %s has no completed turns with timestamps", checkpointID)
	}

	fmt.Fprintf(w, "Checkpoint: %s\n", checkpointID)
	fmt.Fprintln(w, "Turn durations:")
	for _, turn := range turns {
		prompt := stringutil.CollapseWhitespace(turn.prompt)
		prompt = stringutil.TruncateRunes(prompt, profilePromptMaxLength, "...")
		fmt.Fprintf(w, "- %8s  %s\n", turn.duration.Round(time.Second).String(), prompt)
	}

	return nil
}

func profileTemporaryCheckpoint(ctx context.Context, w io.Writer, repo *git.Repository, store *checkpoint.GitStore, shaPrefix string) (bool, error) {
	tempCheckpoints, err := store.ListAllTemporaryCheckpoints(ctx, "", branchCheckpointsLimit)
	if err != nil {
		return false, nil
	}

	var matches []checkpoint.TemporaryCheckpointInfo
	for _, tc := range tempCheckpoints {
		if strings.HasPrefix(tc.CommitHash.String(), shaPrefix) {
			matches = append(matches, tc)
		}
	}

	switch len(matches) {
	case 0:
		return false, nil
	case 1:
	default:
		examples := make([]string, 0, 5)
		for i := 0; i < len(matches) && i < 5; i++ {
			examples = append(examples, matches[i].CommitHash.String()[:7])
		}
		return false, fmt.Errorf("ambiguous checkpoint prefix %q matches %d temporary checkpoints: %s", shaPrefix, len(matches), strings.Join(examples, ", "))
	}

	tc := matches[0]
	shadowCommit, err := repo.CommitObject(tc.CommitHash)
	if err != nil {
		return false, fmt.Errorf("failed to read temporary checkpoint: %w", err)
	}

	shadowTree, err := shadowCommit.Tree()
	if err != nil {
		return false, fmt.Errorf("failed to read temporary checkpoint tree: %w", err)
	}

	agentType := strategy.ReadAgentTypeFromTree(shadowTree, tc.MetadataDir)
	transcriptBytes, err := store.GetTranscriptFromCommit(ctx, tc.CommitHash, tc.MetadataDir, agentType)
	if err != nil {
		return false, fmt.Errorf("failed to read checkpoint content: %w", err)
	}
	if len(transcriptBytes) == 0 {
		return false, fmt.Errorf("checkpoint %s has no transcript", tc.CommitHash.String()[:7])
	}

	turns, err := profileTurns(transcriptBytes, agentType)
	if err != nil {
		return false, err
	}
	if len(turns) == 0 {
		return false, fmt.Errorf("checkpoint %s has no completed turns with timestamps", tc.CommitHash.String()[:7])
	}

	fmt.Fprintf(w, "Checkpoint: %s [temporary]\n", tc.CommitHash.String()[:7])
	fmt.Fprintf(w, "Session: %s\n", tc.SessionID)
	fmt.Fprintln(w, "Turn durations:")
	for _, turn := range turns {
		prompt := stringutil.CollapseWhitespace(turn.prompt)
		prompt = stringutil.TruncateRunes(prompt, profilePromptMaxLength, "...")
		fmt.Fprintf(w, "- %8s  %s\n", turn.duration.Round(time.Second).String(), prompt)
	}

	return true, nil
}

func profileTurns(transcriptBytes []byte, agentType types.AgentType) ([]turnDuration, error) {
	messages, err := extractTimedMessages(transcriptBytes, agentType)
	if err != nil {
		return nil, err
	}

	var turns []turnDuration
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.role != transcript.TypeUser || msg.prompt == "" || msg.timestamp.IsZero() {
			continue
		}

		nextUserIdx := -1
		for j := i + 1; j < len(messages); j++ {
			if messages[j].role == transcript.TypeUser {
				nextUserIdx = j
				break
			}
		}
		if nextUserIdx == -1 {
			continue
		}

		lastAssistant := time.Time{}
		for j := i + 1; j < nextUserIdx; j++ {
			candidate := messages[j]
			if candidate.role == transcript.TypeAssistant && !candidate.timestamp.IsZero() {
				lastAssistant = candidate.timestamp
			}
		}
		if lastAssistant.IsZero() || lastAssistant.Before(msg.timestamp) {
			continue
		}

		turns = append(turns, turnDuration{
			prompt:   msg.prompt,
			duration: lastAssistant.Sub(msg.timestamp),
		})
	}

	return turns, nil
}

func extractTimedMessages(transcriptBytes []byte, agentType types.AgentType) ([]timedMessage, error) {
	tryParsers := func(parsers ...func([]byte) ([]timedMessage, error)) ([]timedMessage, error) {
		var lastErr error
		for _, parser := range parsers {
			messages, err := parser(transcriptBytes)
			if err != nil {
				lastErr = err
				continue
			}
			if len(messages) > 0 {
				return messages, nil
			}
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, nil
	}

	switch agentType {
	case agent.AgentTypeOpenCode:
		return tryParsers(parseOpenCodeTimedMessages, parseJSONLTimedMessages, parseGeminiTimedMessages)
	case agent.AgentTypeGemini:
		return tryParsers(parseGeminiTimedMessages, parseOpenCodeTimedMessages, parseJSONLTimedMessages)
	case agent.AgentTypeClaudeCode, agent.AgentTypeCursor, agent.AgentTypeFactoryAIDroid:
		return tryParsers(parseJSONLTimedMessages, parseOpenCodeTimedMessages, parseGeminiTimedMessages)
	default:
		// Unknown agent: attempt all known formats, prioritizing OpenCode and Gemini JSON.
		return tryParsers(parseOpenCodeTimedMessages, parseGeminiTimedMessages, parseJSONLTimedMessages)
	}
}

func parseOpenCodeTimedMessages(transcriptBytes []byte) ([]timedMessage, error) {
	var session struct {
		Messages []struct {
			Info struct {
				Role string `json:"role"`
				Time struct {
					Created   int64 `json:"created"`
					Completed int64 `json:"completed"`
				} `json:"time"`
			} `json:"info"`
			Parts []struct {
				Type string `json:"type"`
				Text string `json:"text,omitempty"`
			} `json:"parts"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(transcriptBytes, &session); err != nil {
		return nil, fmt.Errorf("failed to parse transcript: %w", err)
	}

	messages := make([]timedMessage, 0, len(session.Messages))
	for _, msg := range session.Messages {
		role := normalizeRole(msg.Info.Role)
		if role == "" {
			continue
		}

		timestamp := unixTimestampAuto(msg.Info.Time.Created)
		if role == transcript.TypeAssistant && msg.Info.Time.Completed > 0 {
			timestamp = unixTimestampAuto(msg.Info.Time.Completed)
		}

		prompt := ""
		if role == transcript.TypeUser {
			var parts []string
			for _, part := range msg.Parts {
				if part.Type == "text" && part.Text != "" {
					parts = append(parts, part.Text)
				}
			}
			prompt = strings.Join(parts, "\n")
		}

		messages = append(messages, timedMessage{role: role, prompt: prompt, timestamp: timestamp})
	}

	return messages, nil
}

func parseGeminiTimedMessages(transcriptBytes []byte) ([]timedMessage, error) {
	var session struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(transcriptBytes, &session); err != nil {
		return nil, fmt.Errorf("failed to parse transcript: %w", err)
	}

	messages := make([]timedMessage, 0, len(session.Messages))
	for _, raw := range session.Messages {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		msgType := jsonString(msg["type"])
		role := ""
		switch msgType {
		case "user":
			role = transcript.TypeUser
		case "gemini", "assistant":
			role = transcript.TypeAssistant
		}
		if role == "" {
			continue
		}

		timestamp := extractTimestampFromRawMap(msg, role == transcript.TypeAssistant)
		prompt := ""
		if role == transcript.TypeUser {
			prompt = extractGeminiUserPrompt(msg["content"])
		}

		messages = append(messages, timedMessage{role: role, prompt: prompt, timestamp: timestamp})
	}

	return messages, nil
}

func parseJSONLTimedMessages(transcriptBytes []byte) ([]timedMessage, error) {
	reader := bufio.NewReader(bytes.NewReader(transcriptBytes))
	var messages []timedMessage

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read transcript: %w", err)
		}

		if len(bytes.TrimSpace(lineBytes)) > 0 {
			var line map[string]json.RawMessage
			if unmarshalErr := json.Unmarshal(lineBytes, &line); unmarshalErr == nil {
				role := normalizeRole(jsonString(line["type"]))
				if role == "" {
					role = normalizeRole(jsonString(line["role"]))
				}

				if role != "" {
					timestamp := extractTimestampFromRawMap(line, role == transcript.TypeAssistant)
					prompt := ""
					if role == transcript.TypeUser {
						prompt = transcript.ExtractUserContent(line["message"])
					}
					messages = append(messages, timedMessage{role: role, prompt: prompt, timestamp: timestamp})
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	return messages, nil
}

func extractTimestampFromRawMap(line map[string]json.RawMessage, preferCompleted bool) time.Time {
	if t := parseRawTimestamp(line["timestamp"]); !t.IsZero() {
		return t
	}

	if t := parseRawTimestamp(line["created_at"]); !t.IsZero() {
		return t
	}

	if rawTime, ok := line["time"]; ok && len(rawTime) > 0 {
		var tm map[string]json.RawMessage
		if err := json.Unmarshal(rawTime, &tm); err == nil {
			if preferCompleted {
				if t := parseRawTimestamp(tm["completed"]); !t.IsZero() {
					return t
				}
			}
			if t := parseRawTimestamp(tm["created"]); !t.IsZero() {
				return t
			}
			if t := parseRawTimestamp(tm["completed"]); !t.IsZero() {
				return t
			}
		}
	}

	return time.Time{}
}

func parseRawTimestamp(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, str); parseErr == nil {
			return parsed
		}
		if parsed, parseErr := time.Parse(time.RFC3339, str); parseErr == nil {
			return parsed
		}
	}

	var num float64
	if err := json.Unmarshal(raw, &num); err == nil {
		unix := int64(num)
		if unix > 1_000_000_000_000 {
			return time.UnixMilli(unix).UTC()
		}
		if unix > 0 {
			return time.Unix(unix, 0).UTC()
		}
	}

	return time.Time{}
}

func normalizeRole(role string) string {
	switch strings.ToLower(role) {
	case transcript.TypeUser, "human":
		return transcript.TypeUser
	case transcript.TypeAssistant, "gemini":
		return transcript.TypeAssistant
	default:
		return ""
	}
}

func extractGeminiUserPrompt(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}

	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}

	texts := make([]string, 0, len(parts))
	for _, p := range parts {
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}

	return strings.Join(texts, "\n")
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func unixTimestampAuto(ts int64) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	if ts > 1_000_000_000_000 {
		return time.UnixMilli(ts).UTC()
	}
	return time.Unix(ts, 0).UTC()
}
