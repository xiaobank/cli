package memorylooptui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/entireio/cli/cmd/entire/cli/memoryloop"
)

//nolint:recvcheck // bubbletea pattern: pointer receivers for mutation, value for update/view
type historyModel struct {
	state  *memoryloop.State
	styles tuiStyles //nolint:unused // used in later task (history tab implementation)
	width  int
	height int
}

func (m *historyModel) setState(state *memoryloop.State) { m.state = state }
func (m *historyModel) setSize(w, h int)                 { m.width = w; m.height = h }

func (m historyModel) update(_ tea.Msg) (historyModel, tea.Cmd) {
	return m, nil
}

func (m historyModel) view() string {
	if m.state == nil || m.state.Store == nil {
		return "No refresh history. Press R to refresh."
	}
	return fmt.Sprintf("History tab -- %d refreshes (implementation pending)", len(m.state.Store.RefreshHistory))
}
