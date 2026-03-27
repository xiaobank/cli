package skilltui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/entireio/cli/cmd/entire/cli/skilldb"
)

//nolint:recvcheck // bubbletea pattern: pointer receivers for mutation, value for update/view
type frictionModel struct {
	friction []skilldb.FrictionThemeRow
	missing  []skilldb.MissingInstructionRow
	selected int
	scroll   int
	styles   tuiStyles
	width    int
	height   int
}

func (m *frictionModel) setData(friction []skilldb.FrictionThemeRow, missing []skilldb.MissingInstructionRow) {
	m.friction = friction
	m.missing = missing
	m.selected = 0
	m.scroll = 0
}

func (m *frictionModel) setSize(w, h int) { m.width = w; m.height = h }

func (m frictionModel) totalItems() int {
	return len(m.friction) + len(m.missing)
}

//nolint:unparam // tea.Cmd return needed for bubbletea interface consistency
func (m frictionModel) update(msg tea.Msg) (frictionModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	total := m.totalItems()
	switch {
	case key.Matches(keyMsg, pickerKeyMap.Up):
		if m.selected > 0 {
			m.selected--
		}
	case key.Matches(keyMsg, pickerKeyMap.Down):
		if m.selected < total-1 {
			m.selected++
		}
	}

	return m, nil
}

func (m frictionModel) view() string {
	if len(m.friction) == 0 && len(m.missing) == 0 {
		return "  No friction or gaps recorded yet.\n"
	}

	var b strings.Builder
	itemIdx := 0

	// Friction Themes section
	if len(m.friction) > 0 {
		b.WriteString("  ")
		b.WriteString(m.styles.render(m.styles.sectionHeader, "Friction Themes"))
		b.WriteString("\n\n")

		for _, f := range m.friction {
			marker := "  "
			if itemIdx == m.selected {
				marker = m.styles.render(m.styles.selected, "\u25b8 ")
			}

			countStr := m.styles.render(m.styles.bold, fmt.Sprintf("[%dx]", f.Count))
			fmt.Fprintf(&b, "%s%s %s\n", marker, countStr, f.Text)

			if f.Category != "" {
				fmt.Fprintf(&b, "       Category: %s\n", f.Category)
			}
			if len(f.Sessions) > 0 {
				sessStr := strings.Join(f.Sessions, ", ")
				if len(sessStr) > 60 {
					sessStr = sessStr[:57] + "..."
				}
				fmt.Fprintf(&b, "       Sessions: %s\n", sessStr)
			}
			b.WriteString("\n")
			itemIdx++
		}
	}

	// Missing Instructions section
	if len(m.missing) > 0 {
		b.WriteString("  ")
		b.WriteString(m.styles.render(m.styles.sectionHeader, "Missing Instructions"))
		b.WriteString("\n\n")

		for _, mi := range m.missing {
			marker := "  "
			if itemIdx == m.selected {
				marker = m.styles.render(m.styles.selected, "\u25b8 ")
			}

			countStr := m.styles.render(m.styles.bold, fmt.Sprintf("[%dx]", mi.Count))
			fmt.Fprintf(&b, "%s%s %s\n", marker, countStr, mi.Instruction)

			if len(mi.Evidence) > 0 {
				for _, ev := range mi.Evidence {
					if ev != "" {
						evStr := ev
						if len(evStr) > 70 {
							evStr = evStr[:67] + "..."
						}
						fmt.Fprintf(&b, "       Evidence: %q\n", evStr)
					}
				}
			}
			b.WriteString("\n")
			itemIdx++
		}
	}

	return b.String()
}
