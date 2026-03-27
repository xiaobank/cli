package skilltui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/entireio/cli/cmd/entire/cli/skilldb"
)

//nolint:recvcheck // bubbletea pattern: pointer receivers for mutation, value for update/view
type pickerModel struct {
	skills   []skilldb.SkillRow
	stats    map[string]*skilldb.SkillStatsResult
	selected int
	styles   tuiStyles
	width    int
	height   int
}

func newPickerModel(styles tuiStyles) pickerModel {
	return pickerModel{
		styles: styles,
		stats:  make(map[string]*skilldb.SkillStatsResult),
	}
}

func (m *pickerModel) setData(skills []skilldb.SkillRow, stats map[string]*skilldb.SkillStatsResult) {
	m.skills = skills
	m.stats = stats
	m.selected = 0
}

func (m *pickerModel) setSize(w, h int) { m.width = w; m.height = h }

func (m pickerModel) update(msg tea.Msg) (pickerModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(keyMsg, pickerKeyMap.Up):
		if m.selected > 0 {
			m.selected--
		}
	case key.Matches(keyMsg, pickerKeyMap.Down):
		if m.selected < len(m.skills)-1 {
			m.selected++
		}
	case key.Matches(keyMsg, pickerKeyMap.Enter):
		if len(m.skills) > 0 {
			skill := m.skills[m.selected]
			return m, func() tea.Msg { return skillSelectedMsg{skill: skill} }
		}
	}

	return m, nil
}

func (m pickerModel) view() string {
	var b strings.Builder

	b.WriteString(renderPickerHeader(m.styles))
	b.WriteString("\n")

	if len(m.skills) == 0 {
		b.WriteString("  No skills found. Create skill files in .claude/skills/ or .gemini/agents/\n")
		return b.String()
	}

	// Column headers
	header := fmt.Sprintf("  %-20s %-14s %8s %10s %9s", "Name", "Source", "Sessions", "Freq", "Avg Score")
	b.WriteString(m.styles.render(m.styles.dim, header))
	b.WriteString("\n")
	b.WriteString(m.styles.render(m.styles.dim, strings.Repeat("\u2500", min(m.width, 70))))
	b.WriteString("\n")

	for i, skill := range m.skills {
		statsKey := skill.Name + "|" + skill.SourceAgent
		st := m.stats[statsKey]

		marker := "  "
		nameStyle := m.styles.dim
		if i == m.selected {
			marker = m.styles.render(m.styles.selected, "\u25b8 ")
			nameStyle = m.styles.bold
		}

		sessions := 0
		freqStr := m.styles.render(m.styles.dim, "\u2500 0.0/wk")
		scoreStr := "\u2500"
		if st != nil {
			sessions = st.TotalSessions
			freqStr = formatFrequency(m.styles, st.SessionsPerWeek)
			if st.TotalSessions > 0 {
				scoreStr = fmt.Sprintf("%.0f", st.AvgScore)
			}
		}

		line := fmt.Sprintf("%s%-20s %-14s %8d %10s %9s",
			marker,
			m.styles.render(nameStyle, skill.Name),
			skill.SourceAgent,
			sessions,
			freqStr,
			scoreStr,
		)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func formatFrequency(s tuiStyles, perWeek float64) string {
	rate := fmt.Sprintf("%.1f/wk", perWeek)
	switch {
	case perWeek > 1.0:
		return s.render(s.success, "\u25b2") + " " + rate
	case perWeek < 0.5:
		return s.render(s.friction, "\u25bc") + " " + rate
	default:
		return s.render(s.dim, "\u2500") + " " + rate
	}
}
