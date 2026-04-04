package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ModalAction int

const (
	ActionNone ModalAction = iota
	ActionAdd
	ActionRename
	ActionDelete
)

var (
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	guideStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
)

// laneKeyLane maps h/j/k/l/;/H/J/K/L/: to their lane index (0–4).
var laneKeyLane = map[string]int{
	"h": 0, "j": 1, "k": 2, "l": 3, ";": 4,
	"H": 0, "J": 1, "K": 2, "L": 3, ":": 4,
}

// laneKeyShift identifies the shift variants (move up).
var laneKeyShift = map[string]bool{
	"H": true, "J": true, "K": true, "L": true, ":": true,
}

type Model struct {
	session Session
	windows []Window
	lanes   map[string][]Window

	// Column cursor
	colLane   int // 0–4, index into laneOrder
	colWindow int // index into lanes[laneOrder[colLane]]

	// Cut/paste
	cutWinID string

	// Input mode (add / rename)
	inputMode   bool
	inputPrompt string
	inputValue  []rune
	modalAction ModalAction

	// Confirm modal
	modal tea.Model

	// Command file for deferred add-window (when running as a popup)
	commandFile   string
	returnCommand string
	switchCommand string

	// Window to restore on cancel
	initialWinID string

	width  int
	height int
}

func newModel(initialSessID, initialWinID, commandFile, returnCommand, switchCommand string) (Model, error) {
	sess, err := loadSession(initialSessID)
	if err != nil {
		return Model{}, err
	}
	windows, err := loadWindows(initialSessID)
	if err != nil {
		return Model{}, err
	}

	m := Model{
		session:       sess,
		windows:       windows,
		lanes:         groupByLane(windows),
		commandFile:   commandFile,
		returnCommand: returnCommand,
		switchCommand: switchCommand,
		initialWinID:  initialWinID,
		width:         80,
		height:        24,
	}
	m.positionOnWindow(initialWinID)
	return m, nil
}

func (m *Model) positionOnWindow(winID string) {
	for li, key := range laneOrder {
		for wi, w := range m.lanes[key] {
			if w.ID == winID {
				m.colLane = li
				m.colWindow = wi
				return
			}
		}
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case ModalDoneMsg:
		return m.handleModalDone(msg)

	case tea.KeyMsg:
		if m.modal != nil {
			var cmd tea.Cmd
			m.modal, cmd = m.modal.Update(msg)
			return m, cmd
		}
		if m.inputMode {
			return m.handleInputKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleInputKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = false
		m.inputValue = nil
		m.modalAction = ActionNone
	case "enter":
		value := string(m.inputValue)
		m.inputMode = false
		m.inputValue = nil
		return m.handleModalDone(ModalDoneMsg{Value: &value})
	case "backspace", "ctrl+h":
		if len(m.inputValue) > 0 {
			m.inputValue = m.inputValue[:len(m.inputValue)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.inputValue = append(m.inputValue, msg.Runes...)
		}
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m, tea.Quit

	case "esc", "alt+u":
		tmuxRun("switch-client", "-t", m.session.ID+":"+m.initialWinID)
		return m, tea.Quit

	case "alt+o":
		if m.switchCommand != "" && m.commandFile != "" {
			os.WriteFile(m.commandFile, []byte(m.switchCommand+"\n"), 0644)
		}
		return m, tea.Quit

	case "a":
		m.inputMode = true
		m.inputPrompt = "Name"
		m.inputValue = nil
		m.modalAction = ActionAdd
		return m, nil

	case "r":
		if w := m.currentWindow(); w != nil {
			m.inputMode = true
			m.inputPrompt = "Rename"
			m.inputValue = []rune(w.Name)
			m.modalAction = ActionRename
		}
		return m, nil

	case "d":
		if w := m.currentWindow(); w != nil {
			m.modal = newConfirmModal(fmt.Sprintf("Kill window %q?", w.Name))
			m.modalAction = ActionDelete
		}
		return m, nil

	case "c", "x":
		if w := m.currentWindow(); w != nil {
			m.cutWinID = w.ID
		}
		return m, nil

	case "p":
		return m.handlePaste(false)

	case "P":
		return m.handlePaste(true)

	case "down":
		windows := m.lanes[laneOrder[m.colLane]]
		if m.colWindow < len(windows)-1 {
			m.colWindow++
		}
		return m, m.switchToCurrentCmd()

	case "up":
		if m.colWindow > 0 {
			m.colWindow--
		}
		return m, m.switchToCurrentCmd()

	case "right":
		if m.colLane < len(laneOrder)-1 {
			m.colLane++
			m.clampColWindow()
		}
		return m, m.switchToCurrentCmd()

	case "left":
		if m.colLane > 0 {
			m.colLane--
			m.clampColWindow()
		}
		return m, m.switchToCurrentCmd()
	}

	// Lane keys: h/j/k/l/; jump to that lane (or move down if already there).
	// Shift variants H/J/K/L/: move up when already in that lane.
	if laneIdx, ok := laneKeyLane[msg.String()]; ok {
		shift := laneKeyShift[msg.String()]
		if m.colLane == laneIdx {
			windows := m.lanes[laneOrder[laneIdx]]
			if n := len(windows); n > 0 {
				if !shift {
					m.colWindow = (m.colWindow + 1) % n
				} else {
					m.colWindow = (m.colWindow - 1 + n) % n
				}
			}
		} else {
			m.colLane = laneIdx
			windows := m.lanes[laneOrder[laneIdx]]
			if len(windows) > 0 && m.colWindow >= len(windows) {
				m.colWindow = len(windows) - 1
			}
			if len(windows) == 0 {
				m.colWindow = 0
			}
		}
		return m, m.switchToCurrentCmd()
	}

	return m, nil
}

func (m *Model) clampColWindow() {
	windows := m.lanes[laneOrder[m.colLane]]
	if len(windows) > 0 && m.colWindow >= len(windows) {
		m.colWindow = len(windows) - 1
	}
	if len(windows) == 0 {
		m.colWindow = 0
	}
}

func (m Model) handleModalDone(msg ModalDoneMsg) (Model, tea.Cmd) {
	m.modal = nil
	action := m.modalAction
	m.modalAction = ActionNone

	if msg.Value == nil {
		return m, nil // cancelled
	}

	switch action {
	case ActionAdd:
		return m.handleAdd(*msg.Value)
	case ActionRename:
		return m.handleRename(*msg.Value)
	case ActionDelete:
		return m.handleDelete()
	}
	return m, nil
}

func (m Model) handleAdd(name string) (Model, tea.Cmd) {
	if name == "" {
		return m, nil
	}

	laneKey := m.currentLane()

	var targetID string
	position := "a"
	if w := m.currentWindow(); w != nil {
		targetID = w.ID
	} else if len(m.windows) > 0 {
		targetID = m.windows[len(m.windows)-1].ID
	} else {
		targetID = m.session.ID
		position = "b"
	}

	if m.commandFile != "" {
		content := fmt.Sprintf(
			"NEWWIN=$(tmux new-window -%s -t '%s' -n '%s' -c '#{pane_current_path}' -P -F '#{window_id}')\n"+
				"tmux set-window-option -t \"$NEWWIN\" @lane '%s'\n",
			position, targetID, name, laneKey)
		if m.returnCommand != "" {
			content += m.returnCommand + "\n"
		}
		os.WriteFile(m.commandFile, []byte(content), 0644)
		return m, tea.Quit
	}

	out, err := exec.Command("tmux", "new-window",
		"-"+position, "-t", targetID,
		"-n", name, "-c", "#{pane_current_path}",
		"-P", "-F", "#{window_id}").Output()
	if err == nil {
		newWinID := strings.TrimSpace(string(out))
		tmuxRun("set-window-option", "-t", newWinID, "@lane", laneKey)
	}

	m.refresh()
	return m, nil
}

func (m Model) handleRename(name string) (Model, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	w := m.currentWindow()
	if w == nil {
		return m, nil
	}
	winID := w.ID
	tmuxRun("rename-window", "-t", winID, name)
	m.refresh()
	m.positionOnWindow(winID)
	return m, nil
}

func (m Model) handleDelete() (Model, tea.Cmd) {
	w := m.currentWindow()
	if w == nil {
		return m, nil
	}
	winID := w.ID
	laneKey := w.Lane

	nextID := m.findNextWindow(winID, laneKey)
	if nextID != "" {
		tmuxRun("switch-client", "-t", m.session.ID+":"+nextID)
	}

	tmuxRun("kill-window", "-t", winID)
	m.refresh()

	if nextID != "" {
		m.positionOnWindow(nextID)
	}
	return m, nil
}

func (m Model) handlePaste(before bool) (Model, tea.Cmd) {
	if m.cutWinID == "" {
		return m, nil
	}
	target := m.currentWindow()
	laneKey := m.currentLane()

	// Change the cut window's lane.
	tmuxRun("set-window-option", "-t", m.cutWinID, "@lane", laneKey)

	if target != nil && target.ID != m.cutWinID {
		if before {
			// Two-step swap: insert cut after target, then insert target after cut.
			// Step 1: [..., target, cut, ...]
			tmuxRun("move-window", "-a", "-s", m.cutWinID, "-t", target.ID)
			// Step 2: [..., cut, target, ...]
			tmuxRun("move-window", "-a", "-s", target.ID, "-t", m.cutWinID)
		} else {
			// Move immediately after target.
			tmuxRun("move-window", "-a", "-s", m.cutWinID, "-t", target.ID)
		}
	}

	pastedID := m.cutWinID
	m.cutWinID = ""
	m.refresh()
	m.positionOnWindow(pastedID)
	return m, nil
}

func (m Model) findNextWindow(deletedID, preferLane string) string {
	for _, w := range m.lanes[preferLane] {
		if w.ID != deletedID {
			return w.ID
		}
	}
	for _, key := range laneOrder {
		for _, w := range m.lanes[key] {
			if w.ID != deletedID {
				return w.ID
			}
		}
	}
	return ""
}

func (m Model) currentWindow() *Window {
	windows := m.lanes[laneOrder[m.colLane]]
	if m.colWindow >= 0 && m.colWindow < len(windows) {
		return &windows[m.colWindow]
	}
	return nil
}

func (m Model) currentLane() string {
	return laneOrder[m.colLane]
}

func (m Model) switchToCurrentCmd() tea.Cmd {
	w := m.currentWindow()
	if w == nil {
		return nil
	}
	target := m.session.ID + ":" + w.ID
	return func() tea.Msg {
		tmuxRun("switch-client", "-t", target)
		return nil
	}
}

func (m *Model) refresh() {
	windows, _ := loadWindows(m.session.ID)
	m.windows = windows
	m.lanes = groupByLane(windows)
	m.clampColWindow()
	if m.cutWinID != "" {
		found := false
		for _, w := range windows {
			if w.ID == m.cutWinID {
				found = true
				break
			}
		}
		if !found {
			m.cutWinID = ""
		}
	}
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.modal != nil {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			m.modal.View())
	}

	var bar string
	if m.inputMode {
		bar = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(
			dimStyle.Render(m.inputPrompt+": ") + string(m.inputValue) + cursorStyle.Render("█"))
	} else {
		bar = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(
			hintStyle.Render("[a]dd   [r]ename   [d]elete   [c]ut   [p]aste"))
	}

	content := m.viewColumn()
	padding := m.height - (strings.Count(content, "\n") + 1)
	if padding < 1 {
		padding = 1
	}
	return content + strings.Repeat("\n", padding) + bar
}

func (m Model) viewColumn() string {
	// 2-space padding on each side; 5 columns with 2-space inter-column gaps.
	const sidePad = 2
	const gaps = 4 * 2
	colWidth := max(10, (m.width-2*sidePad-gaps)/5)
	contentWidth := 5*colWidth + gaps
	pad := strings.Repeat(" ", sidePad)

	// Header row: lane names side-by-side, each colWidth wide.
	var headerSB strings.Builder
	for li, key := range laneOrder {
		var s lipgloss.Style
		if li == m.colLane {
			s = lipgloss.NewStyle().Width(colWidth).Bold(true)
		} else {
			s = lipgloss.NewStyle().Width(colWidth).Foreground(lipgloss.Color("246"))
		}
		headerSB.WriteString(s.Render(laneDisplayNames[key]))
		if li < len(laneOrder)-1 {
			headerSB.WriteString("  ")
		}
	}

	// Single rule spanning all columns and gaps.
	ruleRow := pad + guideStyle.Render(strings.Repeat("─", contentWidth))

	// Window rows: zip per-lane lines together.
	var colLines [][]string
	maxHeight := 0
	for li, key := range laneOrder {
		lines := m.renderWindowLines(li, key, colWidth)
		colLines = append(colLines, lines)
		if len(lines) > maxHeight {
			maxHeight = len(lines)
		}
	}

	emptyCell := strings.Repeat(" ", colWidth)
	rows := []string{pad + headerSB.String(), ruleRow}
	for row := 0; row < maxHeight; row++ {
		var sb strings.Builder
		for ci, lines := range colLines {
			if row < len(lines) {
				sb.WriteString(lines[row])
			} else {
				sb.WriteString(emptyCell)
			}
			if ci < len(colLines)-1 {
				sb.WriteString("  ")
			}
		}
		rows = append(rows, pad+sb.String())
	}

	return "\n" + strings.Join(rows, "\n")
}

func (m Model) renderWindowLines(laneIdx int, key string, colWidth int) []string {
	windows := m.lanes[key]
	isCursorCol := laneIdx == m.colLane

	plain := lipgloss.NewStyle().Width(colWidth)
	cursor := lipgloss.NewStyle().Width(colWidth).Background(lipgloss.Color("237"))

	if len(windows) == 0 {
		var s lipgloss.Style
		if isCursorCol {
			s = cursor.Foreground(lipgloss.Color("243"))
		} else {
			s = plain.Foreground(lipgloss.Color("243"))
		}
		return []string{s.Render("(empty)")}
	}

	const cutLabel = " (cut)"
	const cutLabelWidth = len(cutLabel)

	var lines []string
	for wi, w := range windows {
		var s lipgloss.Style
		if isCursorCol && wi == m.colWindow {
			s = cursor
		} else {
			s = plain
		}
		var cell string
		if w.ID == m.cutWinID {
			nameWidth := colWidth - cutLabelWidth
			if nameWidth < 1 {
				nameWidth = 1
			}
			cell = s.Render(truncate(w.Name, nameWidth) + dimStyle.Render(cutLabel))
		} else {
			cell = s.Render(truncate(w.Name, colWidth))
		}
		lines = append(lines, cell)
	}
	return lines
}

func truncate(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-1]) + "…"
}
