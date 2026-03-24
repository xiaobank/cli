package cli

import (
	"io"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/termstyle"
)

// statusStyles holds pre-built lipgloss styles and terminal metadata.
// Fields are unexported to keep the rendering API internal to the cli package.
// All logic delegates to the termstyle package.
type statusStyles struct {
	colorEnabled bool
	width        int

	green lipgloss.Style
	red   lipgloss.Style
	gray  lipgloss.Style
	bold  lipgloss.Style
	dim   lipgloss.Style
	agent lipgloss.Style // amber/orange for agent names
	cyan  lipgloss.Style
}

// newStatusStyles creates styles appropriate for the output writer by
// delegating to termstyle.New and mapping the exported fields to the
// unexported ones used throughout the cli package.
func newStatusStyles(w io.Writer) statusStyles {
	ts := termstyle.New(w)
	return statusStyles{
		colorEnabled: ts.ColorEnabled,
		width:        ts.Width,
		green:        ts.Green,
		red:          ts.Red,
		gray:         ts.Gray,
		bold:         ts.Bold,
		dim:          ts.Dim,
		agent:        ts.Agent,
		cyan:         ts.Cyan,
	}
}

// render applies a style to text only when color is enabled.
func (s statusStyles) render(style lipgloss.Style, text string) string {
	return termstyle.Styles{ColorEnabled: s.colorEnabled}.Render(style, text)
}

// shouldUseColor returns true if the writer supports color output.
func shouldUseColor(w io.Writer) bool {
	return termstyle.ShouldUseColor(w)
}

// getTerminalWidth returns the terminal width, capped at 80 with a fallback of 60.
func getTerminalWidth(w io.Writer) int {
	return termstyle.GetTerminalWidth(w)
}

// formatTokenCount formats a token count for display.
// 0 → "0", 500 → "500", 1200 → "1.2k", 14300 → "14.3k"
func formatTokenCount(n int) string {
	return termstyle.FormatTokenCount(n)
}

// totalTokens recursively sums all token fields including subagent tokens.
func totalTokens(tu *agent.TokenUsage) int {
	return termstyle.TotalTokens(tu)
}

// horizontalRule renders a dimmed horizontal rule of the given width.
func (s statusStyles) horizontalRule(width int) string {
	ts := termstyle.Styles{ColorEnabled: s.colorEnabled, Width: width, Dim: s.dim}
	return ts.HorizontalRule()
}

// sectionRule renders a section header like: ── Active Sessions ────────────
func (s statusStyles) sectionRule(label string, width int) string {
	ts := termstyle.Styles{ColorEnabled: s.colorEnabled, Width: width, Dim: s.dim}
	return ts.SectionRule(label)
}

// activeTimeDisplay formats a last interaction time for display.
// Returns "active now" for recent activity (<1min), otherwise "active Xm ago".
func activeTimeDisplay(lastInteraction *time.Time) string {
	if lastInteraction == nil {
		return ""
	}
	d := time.Since(*lastInteraction)
	if d < time.Minute {
		return "active now"
	}
	return "active " + timeAgo(*lastInteraction)
}
