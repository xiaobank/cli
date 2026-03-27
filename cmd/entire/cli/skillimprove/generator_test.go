package skillimprove_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/llmcli"
	"github.com/entireio/cli/cmd/entire/cli/skillimprove"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildCLIResponse wraps a result string in the Claude CLI JSON envelope.
func buildCLIResponse(result string) string {
	b, err := json.Marshal(result)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal result: %v", err))
	}
	return fmt.Sprintf(`{"result":%s}`, string(b))
}

func TestBuildSkillPrompt_ContainsSections(t *testing.T) {
	t.Parallel()

	req := skillimprove.SkillImprovementRequest{
		SkillName:    "deploy",
		SkillPath:    "/skills/deploy/SKILL.md",
		SkillContent: "# Deploy Skill\nRun deploy commands.",
		FrictionThemes: []skillimprove.FrictionTheme{
			{Text: "deploy fails silently", Category: "reliability", Count: 3},
		},
		MissingInstructions: []skillimprove.MissingInstruction{
			{Instruction: "specify environment", Count: 2, Evidence: []string{"forgot staging", "no env flag"}},
		},
		TranscriptExcerpts: []string{"User: deploy broke\nAgent: retrying..."},
		TotalSessions:      10,
		FrictionRate:       30.0,
		AgentBreakdown: []skillimprove.AgentStats{
			{Agent: "Claude Code", SessionCount: 7, AvgScore: 85},
			{Agent: "Gemini CLI", SessionCount: 3, AvgScore: 72},
		},
	}

	prompt := skillimprove.BuildSkillPrompt(req)

	assert.Contains(t, prompt, "<skill_info>")
	assert.Contains(t, prompt, "Name: deploy")
	assert.Contains(t, prompt, "Path: /skills/deploy/SKILL.md")
	assert.Contains(t, prompt, "Total sessions: 10")
	assert.Contains(t, prompt, "Friction rate: 30%")

	assert.Contains(t, prompt, "<skill_content>")
	assert.Contains(t, prompt, "# Deploy Skill")

	assert.Contains(t, prompt, "<friction_patterns>")
	assert.Contains(t, prompt, "[reliability] deploy fails silently (occurred 3 times)")

	assert.Contains(t, prompt, "<missing_instructions>")
	assert.Contains(t, prompt, "specify environment (reported 2 times)")
	assert.Contains(t, prompt, "Evidence: forgot staging; no env flag")

	assert.Contains(t, prompt, "<transcript_excerpts>")
	assert.Contains(t, prompt, "--- Excerpt ---")
	assert.Contains(t, prompt, "User: deploy broke")

	assert.Contains(t, prompt, "<agent_breakdown>")
	assert.Contains(t, prompt, "Claude Code: 7 sessions, avg score 85")
	assert.Contains(t, prompt, "Gemini CLI: 3 sessions, avg score 72")
}

func TestBuildSkillPrompt_OmitsEmptySections(t *testing.T) {
	t.Parallel()

	req := skillimprove.SkillImprovementRequest{
		SkillName:    "build",
		SkillPath:    "/skills/build/SKILL.md",
		SkillContent: "# Build",
		// All optional slices left nil.
		TotalSessions: 5,
		FrictionRate:  0,
	}

	prompt := skillimprove.BuildSkillPrompt(req)

	// Required sections are always present.
	assert.Contains(t, prompt, "<skill_info>")
	assert.Contains(t, prompt, "<skill_content>")

	// Optional sections should be absent.
	assert.NotContains(t, prompt, "<friction_patterns>")
	assert.NotContains(t, prompt, "<missing_instructions>")
	assert.NotContains(t, prompt, "<transcript_excerpts>")
	assert.NotContains(t, prompt, "<agent_breakdown>")
}

func TestGenerate_ParsesResponse(t *testing.T) {
	t.Parallel()

	inner := `{
		"suggestions": [
			{
				"title": "Add error handling docs",
				"description": "Multiple sessions showed confusion about error handling",
				"priority": "high",
				"diff": "--- a/SKILL.md\n+++ b/SKILL.md\n@@ -1,2 +1,3 @@\n # Deploy\n+## Error Handling\n Run deploy.",
				"evidence": ["agent retried 3 times", "user asked about errors"],
				"source_session_ids": ["sess-001"]
			}
		]
	}`

	runner := &llmcli.Runner{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			resp := buildCLIResponse(inner)
			// Use printf to avoid shell quoting issues with single quotes in JSON.
			return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cat <<'ENDOFRESPONSE'\n%s\nENDOFRESPONSE", resp))
		},
	}

	gen := &skillimprove.Generator{Runner: runner}

	result, err := gen.Generate(context.Background(), skillimprove.SkillImprovementRequest{
		SkillName:    "deploy",
		SkillPath:    "/skills/deploy/SKILL.md",
		SkillContent: "# Deploy\nRun deploy.",
		FrictionThemes: []skillimprove.FrictionTheme{
			{Text: "error handling unclear", Category: "docs", Count: 3},
		},
		TotalSessions: 5,
		FrictionRate:  60,
	})

	require.NoError(t, err)
	require.Len(t, result.Suggestions, 1)

	s := result.Suggestions[0]
	assert.Equal(t, "Add error handling docs", s.Title)
	assert.Equal(t, "high", s.Priority)
	assert.Contains(t, s.Diff, "+## Error Handling")
	assert.Equal(t, []string{"agent retried 3 times", "user asked about errors"}, s.Evidence)
	assert.Equal(t, []string{"sess-001"}, s.SourceSessionIDs)
}

func TestGenerate_HandlesEmptySuggestions(t *testing.T) {
	t.Parallel()

	runner := &llmcli.Runner{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			resp := buildCLIResponse(`{"suggestions": []}`)
			return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cat <<'ENDOFRESPONSE'\n%s\nENDOFRESPONSE", resp))
		},
	}

	gen := &skillimprove.Generator{Runner: runner}

	result, err := gen.Generate(context.Background(), skillimprove.SkillImprovementRequest{
		SkillName:    "test",
		SkillPath:    "/skills/test/SKILL.md",
		SkillContent: "# Test",
	})

	require.NoError(t, err)
	assert.Empty(t, result.Suggestions)
}

func TestGenerate_InvalidJSON(t *testing.T) {
	t.Parallel()

	runner := &llmcli.Runner{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			resp := buildCLIResponse("not valid json at all")
			return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cat <<'ENDOFRESPONSE'\n%s\nENDOFRESPONSE", resp))
		},
	}

	gen := &skillimprove.Generator{Runner: runner}

	_, err := gen.Generate(context.Background(), skillimprove.SkillImprovementRequest{
		SkillName:    "test",
		SkillPath:    "/skills/test/SKILL.md",
		SkillContent: "# Test",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse skill improvement suggestions",
		"expected parse error, got: %s", err)
}
