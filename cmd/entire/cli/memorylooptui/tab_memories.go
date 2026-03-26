package memorylooptui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/entireio/cli/cmd/entire/cli/memoryloop"
)

type statusFilter int

const (
	filterAll statusFilter = iota
	filterActive
	filterCandidate
	filterSuppressed
	filterArchived
)

var filterLabels = [5]string{"all", "active", "candidate", "suppressed", "archived"}

//nolint:recvcheck // bubbletea pattern: pointer receivers for mutation, value for update/view
type memoriesModel struct {
	state      *memoryloop.State
	styles     tuiStyles
	width      int
	height     int
	table      table.Model
	filter     statusFilter
	showDetail bool
	searchMode bool
	searchText string
	records    []memoryloop.MemoryRecord // filtered subset
}

func newMemoriesModel(s tuiStyles) memoriesModel {
	columns := []table.Column{
		{Title: " ", Width: 2},
		{Title: "Title", Width: 30},
		{Title: "Kind", Width: 14},
		{Title: "Scope", Width: 5},
		{Title: "Str", Width: 5},
		{Title: "Outcome", Width: 11},
		{Title: "Inj", Width: 4},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Bold(false).
		Faint(true)
	st.Selected = st.Selected.
		Foreground(lipgloss.Color("6")).
		Bold(false)
	t.SetStyles(st)

	return memoriesModel{
		styles: s,
		table:  t,
	}
}

func (m *memoriesModel) setState(state *memoryloop.State) {
	m.state = state
	m.rebuildTable()
}

func (m *memoriesModel) setSize(w, h int) {
	m.width = w
	m.height = h
	// Reserve 3 lines for filter bar + detail pane header area
	tableH := h - 3
	if m.showDetail {
		tableH = h / 2
	}
	if tableH < 3 {
		tableH = 3
	}
	m.table.SetWidth(w)
	m.table.SetHeight(tableH)
}

func (m *memoriesModel) rebuildTable() {
	if m.state == nil || m.state.Store == nil {
		m.records = nil
		m.table.SetRows(nil)
		return
	}

	var filtered []memoryloop.MemoryRecord
	for _, r := range m.state.Store.Records {
		if !m.matchesFilter(r) {
			continue
		}
		if m.searchMode && m.searchText != "" {
			if !strings.Contains(strings.ToLower(r.Title), strings.ToLower(m.searchText)) {
				continue
			}
		}
		filtered = append(filtered, r)
	}
	m.records = filtered

	rows := make([]table.Row, len(filtered))
	for i, r := range filtered {
		rows[i] = table.Row{
			statusDotPlain(r.Status),
			truncate(r.Title, 30),
			string(r.Kind),
			string(r.ScopeKind),
			renderStrengthBar(r.Strength),
			string(r.Outcome),
			strconv.Itoa(r.InjectCount),
		}
	}
	m.table.SetRows(rows)
}

func (m memoriesModel) matchesFilter(r memoryloop.MemoryRecord) bool {
	switch m.filter {
	case filterAll:
		return true
	case filterActive:
		return r.Status == memoryloop.StatusActive
	case filterCandidate:
		return r.Status == memoryloop.StatusCandidate
	case filterSuppressed:
		return r.Status == memoryloop.StatusSuppressed
	case filterArchived:
		return r.Status == memoryloop.StatusArchived
	default:
		return true
	}
}

func (m memoriesModel) selectedRecord() *memoryloop.MemoryRecord {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.records) {
		return nil
	}
	return &m.records[cursor]
}

func (m memoriesModel) update(msg tea.Msg) (memoriesModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.searchMode {
			return m.updateSearch(keyMsg)
		}

		switch {
		case key.Matches(keyMsg, memoriesKeyMap.Filter):
			m.filter = (m.filter + 1) % 5
			m.rebuildTable()
			return m, nil

		case key.Matches(keyMsg, memoriesKeyMap.Search):
			m.searchMode = true
			m.searchText = ""
			return m, nil

		case key.Matches(keyMsg, memoriesKeyMap.Enter):
			m.showDetail = !m.showDetail
			m.setSize(m.width, m.height)
			return m, nil

		case key.Matches(keyMsg, memoriesKeyMap.Activate):
			if r := m.selectedRecord(); r != nil {
				return m, func() tea.Msg {
					return lifecycleActionMsg{id: r.ID, action: memoryloop.LifecycleActionActivate}
				}
			}

		case key.Matches(keyMsg, memoriesKeyMap.Promote):
			if r := m.selectedRecord(); r != nil {
				return m, func() tea.Msg {
					return lifecycleActionMsg{id: r.ID, action: memoryloop.LifecycleActionPromote}
				}
			}

		case key.Matches(keyMsg, memoriesKeyMap.Suppress):
			if r := m.selectedRecord(); r != nil {
				return m, func() tea.Msg {
					return lifecycleActionMsg{id: r.ID, action: memoryloop.LifecycleActionSuppress}
				}
			}

		case key.Matches(keyMsg, memoriesKeyMap.Unsuppress):
			if r := m.selectedRecord(); r != nil {
				return m, func() tea.Msg {
					return lifecycleActionMsg{id: r.ID, action: memoryloop.LifecycleActionUnsuppress}
				}
			}

		case key.Matches(keyMsg, memoriesKeyMap.Archive):
			if r := m.selectedRecord(); r != nil {
				return m, func() tea.Msg {
					return lifecycleActionMsg{id: r.ID, action: memoryloop.LifecycleActionArchive}
				}
			}

		case key.Matches(keyMsg, memoriesKeyMap.Prune):
			return m, func() tea.Msg { return pruneMsg{} }
		}
	}

	// Delegate to table for navigation
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m memoriesModel) updateSearch(msg tea.KeyMsg) (memoriesModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchText = ""
		m.rebuildTable()
		return m, nil
	case "enter":
		m.searchMode = false
		return m, nil
	case "backspace":
		if len(m.searchText) > 0 {
			m.searchText = m.searchText[:len(m.searchText)-1]
			m.rebuildTable()
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.searchText += msg.String()
			m.rebuildTable()
		}
		return m, nil
	}
}

func (m memoriesModel) view() string {
	if m.state == nil || m.state.Store == nil || len(m.state.Store.Records) == 0 {
		return "\n  No memory store found. Switch to History tab and press R to refresh.\n"
	}

	var b strings.Builder

	// Filter bar
	b.WriteString(m.renderFilterBar())
	b.WriteString("\n")

	// Search bar (if active)
	if m.searchMode {
		fmt.Fprintf(&b, "  / %s\u2588\n", m.searchText)
	}

	// Table
	b.WriteString(m.table.View())

	// Detail pane
	if m.showDetail {
		b.WriteString("\n")
		b.WriteString(m.renderDetail())
	}

	return b.String()
}

func (m memoriesModel) renderFilterBar() string {
	counts := m.statusCounts()
	var parts []string
	for i, label := range filterLabels {
		count := counts[i]
		text := fmt.Sprintf("%s (%d)", label, count)
		if statusFilter(i) == m.filter {
			parts = append(parts, m.styles.render(m.styles.title, text))
		} else {
			parts = append(parts, m.styles.render(m.styles.dim, text))
		}
	}
	return "  " + strings.Join(parts, "  ")
}

func (m memoriesModel) statusCounts() [5]int {
	var counts [5]int
	if m.state == nil || m.state.Store == nil {
		return counts
	}
	for _, r := range m.state.Store.Records {
		counts[0]++ // all
		switch r.Status {
		case memoryloop.StatusActive:
			counts[1]++
		case memoryloop.StatusCandidate:
			counts[2]++
		case memoryloop.StatusSuppressed:
			counts[3]++
		case memoryloop.StatusArchived:
			counts[4]++
		}
	}
	return counts
}

func (m memoriesModel) renderDetail() string {
	r := m.selectedRecord()
	if r == nil {
		return ""
	}
	var b strings.Builder
	// Title line
	b.WriteString("  ")
	b.WriteString(m.styles.render(m.styles.title, r.Title))
	b.WriteString("  ")
	b.WriteString(kindStyle(m.styles, r.Kind)(string(r.Kind)))
	b.WriteString("  ")
	b.WriteString(statusDot(m.styles, r.Status))
	b.WriteString(" ")
	b.WriteString(string(r.Status))
	b.WriteString("  ")
	b.WriteString(m.styles.render(m.styles.dim, string(r.ScopeKind)))
	b.WriteString("  ")
	b.WriteString(m.styles.render(m.styles.dim, string(r.Origin)))
	b.WriteString("\n")

	// Body
	if r.Body != "" {
		b.WriteString("  " + r.Body + "\n")
	}

	// Why
	if r.Why != "" {
		b.WriteString("  ")
		b.WriteString(m.styles.render(m.styles.dim, "WHY"))
		b.WriteString("\n")
		b.WriteString("  " + r.Why + "\n")
	}

	// Stats
	b.WriteString("  ")
	stats := fmt.Sprintf("strength: %d/5 · injected: %dx · matched: %dx · last injected: %s · created: %s",
		r.Strength, r.InjectCount, r.MatchCount, timeAgo(r.LastInjectedAt), timeAgo(r.CreatedAt))
	b.WriteString(m.styles.render(m.styles.dim, stats))
	b.WriteString("\n")

	return b.String()
}

func statusDotPlain(status memoryloop.Status) string {
	switch status {
	case memoryloop.StatusActive:
		return "\u25cf"
	case memoryloop.StatusCandidate:
		return "\u25cb"
	case memoryloop.StatusSuppressed:
		return "\u2715"
	case memoryloop.StatusArchived:
		return "\u25cc"
	default:
		return " "
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "\u2026"
}
