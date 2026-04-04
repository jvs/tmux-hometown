package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalDoneMsg is sent by modal types when they finish.
// Value is nil on cancel, non-nil (possibly empty string) on confirm.
type ModalDoneMsg struct{ Value *string }

var (
	modalHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
	return m.message + "  " + modalHintStyle.Render(" [y]es   [n]o")
}
