package memorylooptui

import (
	"fmt"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/memoryloop"
)

var tabNames = [4]string{"Memories", "Injection", "History", "Settings"}

func renderTabBar(s tuiStyles, activeTab int, width int, mode memoryloop.Mode, policy memoryloop.ActivationPolicy) string {
	var b strings.Builder

	for i, name := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if i == activeTab {
			b.WriteString(s.render(s.tabActive, label))
		} else {
			b.WriteString(s.render(s.tabInactive, label))
		}
		if i < len(tabNames)-1 {
			b.WriteString("  ")
		}
	}

	// Right-align mode and policy indicators
	left := b.String()
	modeStr := s.render(s.active, fmt.Sprintf("● %s", mode))
	policyStr := s.render(s.dim, fmt.Sprintf("· %s", policy))
	right := modeStr + " " + policyStr

	// Pad between left and right
	leftLen := lipglossWidth(left)
	rightLen := lipglossWidth(right)
	padding := width - leftLen - rightLen
	if padding < 1 {
		padding = 1
	}

	return left + strings.Repeat(" ", padding) + right
}

func renderStatusBar(s tuiStyles, hints string, info string, width int) string {
	hintsLen := len(hints)
	infoLen := len(info)
	padding := width - hintsLen - infoLen
	if padding < 1 {
		padding = 1
	}
	return s.render(s.statusBar, hints) + strings.Repeat(" ", padding) + s.render(s.dim, info)
}

func renderStrengthBar(strength int) string {
	if strength < 0 {
		strength = 0
	}
	if strength > 5 {
		strength = 5
	}
	filled := strings.Repeat("\u2588", strength)
	empty := strings.Repeat("\u2591", 5-strength)
	return filled + empty
}

func statusDot(s tuiStyles, status memoryloop.Status) string {
	switch status {
	case memoryloop.StatusActive:
		return s.render(s.active, "\u25cf")
	case memoryloop.StatusCandidate:
		return s.render(s.candidate, "\u25cb")
	case memoryloop.StatusSuppressed:
		return s.render(s.suppressed, "\u2715")
	case memoryloop.StatusArchived:
		return s.render(s.archived, "\u25cc")
	default:
		return " "
	}
}

func kindStyle(s tuiStyles, kind memoryloop.Kind) func(string) string {
	switch kind {
	case memoryloop.KindRepoRule, memoryloop.KindWorkflowRule:
		return func(t string) string { return s.render(s.repoRule, t) }
	case memoryloop.KindAgentInstruction:
		return func(t string) string { return s.render(s.agentInstruction, t) }
	case memoryloop.KindSkillPatch:
		return func(t string) string { return s.render(s.skillPatch, t) }
	case memoryloop.KindAntiPattern:
		return func(t string) string { return s.render(s.antiPattern, t) }
	default:
		return func(t string) string { return t }
	}
}

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// lipglossWidth estimates visible width by stripping ANSI sequences.
// For simple use; not a full ANSI parser.
func lipglossWidth(s string) int {
	inEscape := false
	w := 0
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		w++
	}
	return w
}
