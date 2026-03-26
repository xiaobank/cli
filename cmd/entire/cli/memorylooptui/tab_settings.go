package memorylooptui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/entireio/cli/cmd/entire/cli/memoryloop"
)

var modeOrder = []memoryloop.Mode{memoryloop.ModeOff, memoryloop.ModeManual, memoryloop.ModeAuto}
var policyOrder = []memoryloop.ActivationPolicy{memoryloop.ActivationPolicyReview, memoryloop.ActivationPolicyAuto}

//nolint:recvcheck // bubbletea pattern: pointer receivers for mutation, value for update/view
type settingsModel struct {
	state  *memoryloop.State
	styles tuiStyles
	width  int
	height int
}

func (m *settingsModel) setState(state *memoryloop.State) { m.state = state }
func (m *settingsModel) setSize(w, h int)                 { m.width = w; m.height = h }

func (m settingsModel) update(msg tea.Msg) (settingsModel, tea.Cmd) {
	if m.state == nil || m.state.Store == nil {
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	var changed settingsChangedMsg
	hasChange := false

	switch {
	case key.Matches(keyMsg, settingsKeyMap.Mode):
		next := cycleMode(m.state.Store.Mode)
		changed.mode = &next
		hasChange = true

	case key.Matches(keyMsg, settingsKeyMap.Policy):
		next := cyclePolicy(m.state.Store.ActivationPolicy)
		changed.activationPolicy = &next
		hasChange = true

	case key.Matches(keyMsg, settingsKeyMap.MaxUp):
		next := m.state.Store.MaxInjected + 1
		if next > 10 {
			next = 10
		}
		changed.maxInjected = &next
		hasChange = true

	case key.Matches(keyMsg, settingsKeyMap.MaxDown):
		next := m.state.Store.MaxInjected - 1
		if next < 1 {
			next = 1
		}
		changed.maxInjected = &next
		hasChange = true
	}

	if hasChange {
		return m, func() tea.Msg { return changed }
	}
	return m, nil
}

func (m settingsModel) view() string {
	if m.state == nil || m.state.Store == nil {
		return "\n  No settings available.\n"
	}
	store := m.state.Store

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245")).
		Padding(0, 1).
		Width(m.width - 2)

	// Chip styles for selected vs unselected options
	selectedChip := lipgloss.NewStyle().
		Background(lipgloss.Color("214")).
		Foreground(lipgloss.Color("0")).
		Bold(true).
		Padding(0, 1)
	unselectedChip := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Padding(0, 1)

	var b strings.Builder
	b.WriteString("\n")

	// Mode card
	{
		var c strings.Builder
		c.WriteString(m.styles.render(m.styles.bold, "Mode"))
		c.WriteString("  ")
		c.WriteString(m.styles.render(m.styles.dim, "Controls whether active memories inject into prompts"))
		c.WriteString("\n")
		for _, mode := range modeOrder {
			label := string(mode)
			if mode == store.Mode {
				c.WriteString(selectedChip.Render(label))
			} else {
				c.WriteString(unselectedChip.Render(label))
			}
			c.WriteString(" ")
		}
		b.WriteString(cardStyle.Render(c.String()))
		b.WriteString("\n")
	}

	// Policy card
	{
		var c strings.Builder
		c.WriteString(m.styles.render(m.styles.bold, "Activation Policy"))
		c.WriteString("  ")
		c.WriteString(m.styles.render(m.styles.dim, "What happens to newly generated memories"))
		c.WriteString("\n")
		for _, pol := range policyOrder {
			label := string(pol)
			if pol == store.ActivationPolicy {
				c.WriteString(selectedChip.Render(label))
			} else {
				c.WriteString(unselectedChip.Render(label))
			}
			c.WriteString(" ")
		}
		b.WriteString(cardStyle.Render(c.String()))
		b.WriteString("\n")
	}

	// Max Injected card
	{
		var c strings.Builder
		c.WriteString(m.styles.render(m.styles.bold, "Max Injected"))
		c.WriteString("  ")
		c.WriteString(m.styles.render(m.styles.dim, "Maximum memories per prompt injection"))
		c.WriteString("\n")
		numStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)
		fmt.Fprintf(&c, "  ◀  %s  ▶", numStyle.Render(fmt.Sprintf(" %d ", store.MaxInjected)))
		b.WriteString(cardStyle.Render(c.String()))
		b.WriteString("\n")
	}

	// Injection status card
	{
		var c strings.Builder
		c.WriteString(m.styles.render(m.styles.bold, "Injection"))
		c.WriteString("  ")
		if store.InjectionEnabled {
			c.WriteString(m.styles.render(m.styles.active, "● enabled"))
		} else {
			c.WriteString(m.styles.render(m.styles.suppressed, "○ disabled"))
		}
		b.WriteString(cardStyle.Render(c.String()))
		b.WriteString("\n")
	}

	// Stats
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(m.styles.render(m.styles.dim,
		fmt.Sprintf("Last refresh: %s  ·  Store version: %d  ·  Source window: %d sessions",
			timeAgo(store.GeneratedAt), store.Version, store.SourceWindow)))
	b.WriteString("\n")

	return b.String()
}

func cycleMode(current memoryloop.Mode) memoryloop.Mode {
	for i, m := range modeOrder {
		if m == current {
			return modeOrder[(i+1)%len(modeOrder)]
		}
	}
	return memoryloop.ModeOff
}

func cyclePolicy(current memoryloop.ActivationPolicy) memoryloop.ActivationPolicy {
	for i, p := range policyOrder {
		if p == current {
			return policyOrder[(i+1)%len(policyOrder)]
		}
	}
	return memoryloop.ActivationPolicyReview
}
