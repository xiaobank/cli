package memorylooptui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/entireio/cli/cmd/entire/cli/memoryloop"
)

//nolint:recvcheck // bubbletea pattern: pointer receivers for mutation, value for update/view
type injectionModel struct {
	state  *memoryloop.State
	styles tuiStyles //nolint:unused // used in later task (injection tab implementation)
	width  int
	height int
}

func (m *injectionModel) setState(state *memoryloop.State) { m.state = state }
func (m *injectionModel) setSize(w, h int)                 { m.width = w; m.height = h }

func (m injectionModel) update(_ tea.Msg) (injectionModel, tea.Cmd) {
	return m, nil
}

func (m injectionModel) view() string {
	if m.state == nil {
		return "Loading..."
	}
	return fmt.Sprintf("Injection tab -- %d logs (implementation pending)", len(m.state.InjectionLogs))
}
