package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalDoneMsg is sent by modal types when they finish.
// Value is nil on cancel, non-nil (possibly empty string) on confirm.
type ModalDoneMsg struct{ Value *string }

var (
	modalBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Width(52)
	modalHintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	modalCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
)

// ConfirmModal shows a message and waits for y/Enter or Escape/n.
type ConfirmModal struct {
	message string
}

func newConfirmModal(message string) ConfirmModal {
	return ConfirmModal{message: message}
}

func (m ConfirmModal) Init() tea.Cmd { return nil }

func (m ConfirmModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y", "enter":
		v := ""
		return m, func() tea.Msg { return ModalDoneMsg{Value: &v} }
	case "esc", "n", "N":
		return m, func() tea.Msg { return ModalDoneMsg{} }
	}
	return m, nil
}

func (m ConfirmModal) View() string {
	content := m.message + "\n\n" + modalHintStyle.Render("[y] confirm  [Esc] cancel")
	return modalBoxStyle.Render(content)
}
