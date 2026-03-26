package memorylooptui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/entireio/cli/cmd/entire/cli/memoryloop"
)

//nolint:recvcheck // bubbletea pattern: pointer receivers for mutation, value for update/view
type settingsModel struct {
	state  *memoryloop.State
	styles tuiStyles //nolint:unused // used in later task (settings tab implementation)
	width  int
	height int
}

func (m *settingsModel) setState(state *memoryloop.State) { m.state = state }
func (m *settingsModel) setSize(w, h int)                 { m.width = w; m.height = h }

func (m settingsModel) update(_ tea.Msg) (settingsModel, tea.Cmd) {
	return m, nil
}

func (m settingsModel) view() string {
	if m.state == nil || m.state.Store == nil {
		return "No settings available."
	}
	return fmt.Sprintf("Settings -- mode: %s · policy: %s · max: %d",
		m.state.Store.Mode, m.state.Store.ActivationPolicy, m.state.Store.MaxInjected)
}
