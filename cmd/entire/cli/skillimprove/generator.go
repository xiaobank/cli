// Package skillimprove generates AI-powered improvement suggestions for skill
// files and applies unified diffs to update them.
package skillimprove

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/llmcli"
)

// SkillImprovementRequest contains the data needed to generate improvement
// suggestions for a single skill file.
type SkillImprovementRequest struct {
	SkillName           string
	SkillPath           string
	SkillContent        string
	FrictionThemes      []FrictionTheme
	MissingInstructions []MissingInstruction
	TranscriptExcerpts  []string
	TotalSessions       int
	FrictionRate        float64
	AgentBreakdown      []AgentStats
}

// FrictionTheme represents a categorized friction pattern observed across sessions.
type FrictionTheme struct {
	Text     string
	Category string
	Count    int
}

// MissingInstruction represents an instruction the agent needed but was not
// present in the skill file.
type MissingInstruction struct {
	Instruction string
	Count       int
	Evidence    []string
}

// AgentStats holds per-agent session statistics for a skill.
type AgentStats struct {
	Agent        string
	SessionCount int
	AvgScore     float64
}

// SkillSuggestion is a single actionable improvement for a skill file.
type SkillSuggestion struct {
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Priority         string   `json:"priority"`
	Diff             string   `json:"diff"`
	Evidence         []string `json:"evidence"`
	SourceSessionIDs []string `json:"source_session_ids,omitempty"`
}

// GenerateResult holds the suggestions and token usage from a Generate call.
type GenerateResult struct {
	Suggestions []SkillSuggestion
	Usage       *llmcli.UsageInfo
}

// Generator produces skill improvement suggestions via the Claude CLI.
type Generator struct {
	Runner *llmcli.Runner
}

// generateResponse is the expected JSON structure returned by the Claude CLI.
type generateResponse struct {
	Suggestions []SkillSuggestion `json:"suggestions"`
}

// Generate produces improvement suggestions for a skill file based on usage data.
func (g *Generator) Generate(ctx context.Context, req SkillImprovementRequest) (*GenerateResult, error) {
	prompt := BuildSkillPrompt(req)

	if g.Runner == nil {
		g.Runner = &llmcli.Runner{}
	}

	raw, usage, err := g.Runner.Execute(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to execute skill improvement prompt: %w", err)
	}

	var resp generateResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse skill improvement suggestions: %w", err)
	}

	return &GenerateResult{Suggestions: resp.Suggestions, Usage: usage}, nil
}

// BuildSkillPrompt constructs the prompt for the Claude CLI.
// All untrusted content (skill content, friction text, transcript excerpts) is
// wrapped in XML tags to prevent prompt injection.
func BuildSkillPrompt(req SkillImprovementRequest) string {
	var sb strings.Builder

	sb.WriteString(`You are a skill improvement analyst. Analyze the usage data for a CLI skill and suggest specific improvements to the skill file.

`)

	// Skill info section (always present).
	sb.WriteString("<skill_info>\n")
	fmt.Fprintf(&sb, "Name: %s\n", req.SkillName)
	fmt.Fprintf(&sb, "Path: %s\n", req.SkillPath)
	fmt.Fprintf(&sb, "Total sessions: %d\n", req.TotalSessions)
	fmt.Fprintf(&sb, "Friction rate: %.0f%%\n", req.FrictionRate)
	sb.WriteString("</skill_info>\n\n")

	// Skill content section (always present).
	sb.WriteString("<skill_content>\n")
	sb.WriteString(req.SkillContent)
	sb.WriteString("\n</skill_content>\n\n")

	// Friction patterns section (omitted when empty).
	if len(req.FrictionThemes) > 0 {
		sb.WriteString("<friction_patterns>\n")
		for _, theme := range req.FrictionThemes {
			fmt.Fprintf(&sb, "- [%s] %s (occurred %d times)\n", theme.Category, theme.Text, theme.Count)
		}
		sb.WriteString("</friction_patterns>\n\n")
	}

	// Missing instructions section (omitted when empty).
	if len(req.MissingInstructions) > 0 {
		sb.WriteString("<missing_instructions>\n")
		for _, mi := range req.MissingInstructions {
			fmt.Fprintf(&sb, "- %s (reported %d times)\n", mi.Instruction, mi.Count)
			if len(mi.Evidence) > 0 {
				fmt.Fprintf(&sb, "  Evidence: %s\n", strings.Join(mi.Evidence, "; "))
			}
		}
		sb.WriteString("</missing_instructions>\n\n")
	}

	// Transcript excerpts section (omitted when empty).
	if len(req.TranscriptExcerpts) > 0 {
		sb.WriteString("<transcript_excerpts>\n")
		for _, excerpt := range req.TranscriptExcerpts {
			sb.WriteString("--- Excerpt ---\n")
			sb.WriteString(excerpt)
			sb.WriteString("\n")
		}
		sb.WriteString("</transcript_excerpts>\n\n")
	}

	// Agent breakdown section (omitted when empty).
	if len(req.AgentBreakdown) > 0 {
		sb.WriteString("<agent_breakdown>\n")
		for _, a := range req.AgentBreakdown {
			fmt.Fprintf(&sb, "- %s: %d sessions, avg score %.0f\n", a.Agent, a.SessionCount, a.AvgScore)
		}
		sb.WriteString("</agent_breakdown>\n\n")
	}

	sb.WriteString(`Respond with JSON only, no markdown fencing:
{
  "suggestions": [
    {
      "title": "Short improvement title",
      "description": "Why this improvement helps, based on evidence",
      "priority": "high|medium|low",
      "diff": "unified diff against the skill file",
      "evidence": ["quote from transcript or friction item"],
      "source_session_ids": ["session-id-1"]
    }
  ]
}

Rules:
- Generate 1-5 suggestions, prioritized by impact
- Each diff must be a valid unified diff that can be applied to the skill file
- High priority: friction occurring 3+ times or missing instructions reported 3+ times
- Medium priority: friction occurring 2 times
- Low priority: single-occurrence improvements or style suggestions
- Focus on actionable changes to the skill file content, not structural changes
`)

	return sb.String()
}
