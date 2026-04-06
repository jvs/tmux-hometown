package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type SlotsModel struct {
	slots map[int][]Session

	// Column cursor
	colSlot int // 0–4, index into slotKeys
	colRow  int // index into slots[colSlot]

	// Cut/paste
	cutSessID string

	// Input mode (add / rename)
	inputMode   bool
	inputPrompt string
	inputValue  []rune
	modalAction ModalAction

	// Confirm modal
	modal tea.Model

	// Assign-key prompt (shown when current session has no slot key)
	promptMode bool

	// Command file for deferred operations
	commandFile        string
	returnView         string
	activationKey      string
	shiftActivationKey string
	cyclePattern       string

	// Session to restore on cancel.
	initialSessID   string
	initialSessName string

	width  int
	height int
}

func newSlotsModel(initialSessID, commandFile, returnView, activationKey, shiftActivationKey, cyclePattern string) (SlotsModel, error) {
	_, hasSlot := getSessionSlotKey(initialSessID)
	promptMode := !hasSlot &&
		tmuxGetSessionOption(initialSessID, "@hometown_slot_never") != "1"

	sess, _ := loadSession(initialSessID)

	m := SlotsModel{
		slots:              groupBySlot(),
		promptMode:         promptMode,
		commandFile:        commandFile,
		returnView:         returnView,
		activationKey:      activationKey,
		shiftActivationKey: shiftActivationKey,
		cyclePattern:       cyclePattern,
		initialSessID:      initialSessID,
		initialSessName:    sess.Name,
		width:              80,
		height:             24,
	}
	m.positionOnSession(initialSessID)
	return m, nil
}

func (m *SlotsModel) positionOnSession(sessID string) {
	for i := range slotKeys {
		for ri, s := range m.slots[i] {
			if s.ID == sessID {
				m.colSlot = i
				m.colRow = ri
				return
			}
		}
	}
}

func (m SlotsModel) Init() tea.Cmd { return nil }

func (m SlotsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.promptMode {
			return m.handlePromptKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m SlotsModel) handleInputKey(msg tea.KeyMsg) (SlotsModel, tea.Cmd) {
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
	case " ":
		m.inputValue = append(m.inputValue, ' ')
	default:
		if msg.Type == tea.KeyRunes {
			m.inputValue = append(m.inputValue, msg.Runes...)
		}
	}
	return m, nil
}

func (m SlotsModel) handlePromptKey(msg tea.KeyMsg) (SlotsModel, tea.Cmd) {
	switch msg.String() {
	case "s":
		m.promptMode = false
		return m, nil
	case "n":
		tmuxSetSessionOption(m.initialSessID, "@hometown_slot_never", "1")
		m.promptMode = false
		return m, nil
	case "esc":
		if m.initialSessID != "" {
			tmuxRun("switch-client", "-t", m.initialSessID)
		}
		return m, tea.Quit
	case "shift+enter":
		if m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-windows\n"), 0644)
		}
		return m, tea.Quit
	}
	key := msg.String()
	if key == "alt+"+m.shiftActivationKey {
		if m.initialSessID != "" {
			tmuxRun("switch-client", "-t", m.initialSessID)
		}
		return m, tea.Quit
	}
	if key == "alt+"+m.activationKey {
		if m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-windows\n"), 0644)
		}
		return m, tea.Quit
	}
	// Activation key / tab: cycle through popups.
	if key == m.activationKey || key == "tab" {
		return m, cyclePopup("sessions", m.cyclePattern, m.commandFile, true)
	}
	if key == m.shiftActivationKey || key == "shift+tab" {
		return m, cyclePopup("sessions", m.cyclePattern, m.commandFile, false)
	}
	// h/j/k/l/; assign the selected key to the current session.
	for i, sKey := range slotKeys {
		if key == sKey {
			setSessionSlotKey(m.initialSessID, i)
			m.slots = groupBySlot()
			m.positionOnSession(m.initialSessID)
			m.promptMode = false
			return m, nil
		}
	}
	return m, nil
}

func (m SlotsModel) handleKey(msg tea.KeyMsg) (SlotsModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.currentSession() == nil {
			return m.handleEnterEmpty()
		}
		if s := m.currentSession(); s != nil {
			recordActiveWindowVisit(s.ID)
		}
		return m, tea.Quit

	case "esc":
		if m.initialSessID != "" {
			tmuxRun("switch-client", "-t", m.initialSessID)
		}
		return m, tea.Quit

	case "shift+enter":
		if m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-windows\n"), 0644)
		}
		return m, tea.Quit

	case "down", "j":
		sessions := m.slots[m.colSlot]
		if m.colRow < len(sessions)-1 {
			m.colRow++
		} else if m.colSlot < len(slotKeys)-1 {
			m.colSlot++
			m.colRow = 0
		}
		return m, m.switchToCurrentCmd()

	case "up", "k":
		if m.colRow > 0 {
			m.colRow--
		} else if m.colSlot > 0 {
			m.colSlot--
			sessions := m.slots[m.colSlot]
			if len(sessions) > 0 {
				m.colRow = len(sessions) - 1
			} else {
				m.colRow = 0
			}
		}
		return m, m.switchToCurrentCmd()

	case "right", "l":
		if m.colSlot < len(slotKeys)-1 {
			m.colSlot++
			m.clampColRow()
		}
		return m, m.switchToCurrentCmd()

	case "left", "h":
		if m.colSlot > 0 {
			m.colSlot--
			m.clampColRow()
		}
		return m, m.switchToCurrentCmd()
	}

	if ctrl, ok := resolvedCtrlFor[msg.String()]; ok {
		switch ctrl {
		case CtrlAdd:
			m.inputMode = true
			m.inputPrompt = "Name"
			m.inputValue = nil
			m.modalAction = ActionAdd
			return m, nil
		case CtrlRename:
			if s := m.currentSession(); s != nil {
				m.inputMode = true
				m.inputPrompt = "Rename"
				m.inputValue = []rune(s.Name)
				m.modalAction = ActionRename
			}
			return m, nil
		case CtrlCut:
			if s := m.currentSession(); s != nil {
				m.cutSessID = s.ID
			}
			return m, nil
		case CtrlPaste:
			return m.handlePaste()
		case CtrlHide:
			if s := m.currentSession(); s != nil {
				clearSlotForSession(s.ID)
				m.refresh()
			}
			return m, nil
		case CtrlKill:
			if s := m.currentSession(); s != nil {
				m.modal = newConfirmModal(fmt.Sprintf("Kill session %q?", s.Name))
				m.modalAction = ActionDelete
			}
			return m, nil
		}
	}

	key := msg.String()
	if key == "alt+"+m.shiftActivationKey {
		if m.initialSessID != "" {
			tmuxRun("switch-client", "-t", m.initialSessID)
		}
		return m, tea.Quit
	}
	if key == "alt+"+m.activationKey {
		if m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-windows\n"), 0644)
		}
		return m, tea.Quit
	}
	// Activation key / tab: cycle through popups.
	if key == m.activationKey || key == "tab" {
		return m, cyclePopup("sessions", m.cyclePattern, m.commandFile, true)
	}
	if key == m.shiftActivationKey || key == "shift+tab" {
		return m, cyclePopup("sessions", m.cyclePattern, m.commandFile, false)
	}

	// alt+slot-key — jump to that slot column (or cycle within if already there).
	if slotIdx, ok := altSlotKey[key]; ok {
		if m.colSlot == slotIdx {
			sessions := m.slots[slotIdx]
			if n := len(sessions); n > 0 {
				m.colRow = (m.colRow + 1) % n
			}
		} else {
			m.colSlot = slotIdx
			sessions := m.slots[slotIdx]
			if len(sessions) > 0 && m.colRow >= len(sessions) {
				m.colRow = len(sessions) - 1
			}
			if len(sessions) == 0 {
				m.colRow = 0
			}
		}
		return m, m.switchToCurrentCmd()
	}

	return m, nil
}

func (m *SlotsModel) clampColRow() {
	sessions := m.slots[m.colSlot]
	if len(sessions) > 0 && m.colRow >= len(sessions) {
		m.colRow = len(sessions) - 1
	}
	if len(sessions) == 0 {
		m.colRow = 0
	}
}

func (m SlotsModel) handleModalDone(msg ModalDoneMsg) (SlotsModel, tea.Cmd) {
	m.modal = nil
	action := m.modalAction
	m.modalAction = ActionNone

	if msg.Value == nil {
		return m, nil
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

func (m SlotsModel) handleAdd(name string) (SlotsModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	key := m.colSlot

	if m.commandFile != "" {
		exe, _ := os.Executable()
		content := fmt.Sprintf(
			"NEWSESS=$(tmux new-session -d -s %s -P -F '#{session_id}' 2>/dev/null || tmux new-session -d -P -F '#{session_id}')\n"+
				"tmux set-option -t \"$NEWSESS\" @hometown_slot %s\n"+
				"NEWWIN=$(tmux display-message -t \"$NEWSESS\" -p '#{window_id}')\n"+
				"%s record-window-visit \"$NEWWIN\"\n",
			shellSingleQuote(name), storeIndex(key), exe)
		if m.returnView != "" {
			content += exe + " show-" + m.returnView + "\n"
		}
		os.WriteFile(m.commandFile, []byte(content), 0644)
		return m, tea.Quit
	}

	out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-P", "-F", "#{session_id}").Output()
	if err != nil {
		out, err = exec.Command("tmux", "new-session", "-d", "-P", "-F", "#{session_id}").Output()
		if err != nil {
			return m, nil
		}
	}
	newSessID := strings.TrimSpace(string(out))
	setSessionSlotKey(newSessID, key)
	recordActiveWindowVisit(newSessID)
	m.refresh()
	return m, nil
}

func (m SlotsModel) handleEnterEmpty() (SlotsModel, tea.Cmd) {
	key := m.colSlot

	if m.commandFile != "" {
		exe, _ := os.Executable()
		name := "Session " + slotIndexName(key)
		content := fmt.Sprintf(
			"NEWSESS=$(tmux new-session -d -s %s -P -F '#{session_id}' 2>/dev/null || tmux new-session -d -P -F '#{session_id}')\n"+
				"tmux set-option -t \"$NEWSESS\" @hometown_slot %s\n"+
				"tmux set-window-option -t \"$NEWSESS\" @hometown_lane %s\n"+
				"tmux switch-client -t \"$NEWSESS\"\n"+
				"NEWWIN=$(tmux display-message -t \"$NEWSESS\" -p '#{window_id}')\n"+
				"%s record-window-visit \"$NEWWIN\"\n",
			shellSingleQuote(name), storeIndex(key), storeIndex(1), exe)
		os.WriteFile(m.commandFile, []byte(content), 0644)
		return m, tea.Quit
	}

	newSessID, err := newSlotSession(key)
	if err != nil {
		return m, nil
	}
	setSessionSlotKey(newSessID, key)
	tmuxRun("switch-client", "-t", newSessID)
	recordActiveWindowVisit(newSessID)
	return m, tea.Quit
}

func (m SlotsModel) handleRename(name string) (SlotsModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	s := m.currentSession()
	if s == nil {
		return m, nil
	}
	sessID := s.ID
	tmuxRun("rename-session", "-t", sessID, name)
	m.refresh()
	m.positionOnSession(sessID)
	return m, nil
}

func (m SlotsModel) handleDelete() (SlotsModel, tea.Cmd) {
	s := m.currentSession()
	if s == nil {
		return m, nil
	}
	sessID := s.ID

	all, _ := listAllSessions()

	// If this is the only tmux session, killing it exits tmux.
	if len(all) <= 1 {
		tmuxRun("kill-session", "-t", sessID)
		return m, tea.Quit
	}

	fallbackID := m.findFallbackSession(sessID, all)
	tmuxRun("switch-client", "-t", fallbackID)
	tmuxRun("kill-session", "-t", sessID)
	m.refresh()
	return m, nil
}

func (m SlotsModel) findFallbackSession(deletedID string, all []Session) string {
	// 1. Another session in the same column.
	for _, s := range m.slots[m.colSlot] {
		if s.ID != deletedID {
			return s.ID
		}
	}

	// 2. Any session with a key assigned.
	for i := range slotKeys {
		for _, s := range m.slots[i] {
			if s.ID != deletedID {
				return s.ID
			}
		}
	}

	// 3. Most recently hometown-visited session (any).
	if windows, err := listAllWindowVisits(); err == nil {
		var best *visitedWindow
		for i := range windows {
			w := &windows[i]
			if w.SessionID == deletedID {
				continue
			}
			if best == nil || w.Visited > best.Visited {
				best = &windows[i]
			}
		}
		if best != nil {
			return best.SessionID
		}
	}

	// 4. First available session.
	for _, s := range all {
		if s.ID != deletedID {
			return s.ID
		}
	}

	return ""
}

func (m SlotsModel) handlePaste() (SlotsModel, tea.Cmd) {
	if m.cutSessID == "" {
		return m, nil
	}
	setSessionSlotKey(m.cutSessID, m.colSlot)
	pastedID := m.cutSessID
	m.cutSessID = ""
	m.refresh()
	m.positionOnSession(pastedID)
	return m, nil
}

func (m SlotsModel) currentSession() *Session {
	sessions := m.slots[m.colSlot]
	if m.colRow >= 0 && m.colRow < len(sessions) {
		s := sessions[m.colRow]
		return &s
	}
	return nil
}

func (m SlotsModel) switchToCurrentCmd() tea.Cmd {
	s := m.currentSession()
	if s == nil {
		return nil
	}
	sessID := s.ID
	return func() tea.Msg {
		tmuxRun("switch-client", "-t", sessID)
		return nil
	}
}

func (m *SlotsModel) refresh() {
	m.slots = groupBySlot()
	m.clampColRow()
	if m.cutSessID != "" {
		found := false
		for _, sessions := range m.slots {
			for _, s := range sessions {
				if s.ID == m.cutSessID {
					found = true
					break
				}
			}
		}
		if !found {
			m.cutSessID = ""
		}
	}
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m SlotsModel) jColumnOffset() int {
	const sidePad = 2
	const gaps = 4 * 2
	colWidth := max(10, (m.width-2*sidePad-gaps)/5)
	return sidePad + colWidth + 2
}

func (m SlotsModel) View() string {
	if m.promptMode {
		return m.viewPrompt()
	}

	pad := strings.Repeat(" ", m.jColumnOffset())
	var bar string
	switch {
	case m.modal != nil:
		bar = pad + m.modal.View()
	case m.inputMode:
		bar = pad + dimStyle.Render(m.inputPrompt+": ") + string(m.inputValue) + cursorStyle.Render("█")
	default:
		bar = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(
			hintStyle.Render(resolvedHintBar))
	}

	content := m.viewGrid()
	padding := m.height - (strings.Count(content, "\n") + 1)
	if padding < 1 {
		padding = 1
	}
	return content + strings.Repeat("\n", padding) + bar
}

func (m SlotsModel) viewPrompt() string {
	question := lipgloss.NewStyle().Render(
		fmt.Sprintf("Assign a slot to session %q?", m.initialSessName))
	options := hintStyle.Render(slotPromptKeyList() + "  [s]kip  [n]ever")
	line := question + "  " + options
	centered := lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(line)
	top := strings.Repeat("\n", m.height/2-1)
	return top + centered
}

func (m SlotsModel) viewGrid() string {
	const sidePad = 2
	const gaps = 4 * 2
	colWidth := max(10, (m.width-2*sidePad-gaps)/5)
	contentWidth := 5*colWidth + gaps
	pad := strings.Repeat(" ", sidePad)

	var headerSB strings.Builder
	for li := range slotKeys {
		var s lipgloss.Style
		if li == m.colSlot {
			s = lipgloss.NewStyle().Width(colWidth).Bold(true)
		} else {
			s = lipgloss.NewStyle().Width(colWidth).Foreground(lipgloss.Color("246"))
		}
		headerSB.WriteString(s.Render(slotIndexDisplay(li)))
		if li < len(slotKeys)-1 {
			headerSB.WriteString("  ")
		}
	}

	ruleRow := pad + guideStyle.Render(strings.Repeat("─", contentWidth))

	var colLines [][]string
	maxHeight := 0
	for li := range slotKeys {
		lines := m.renderSessionLines(li, colWidth)
		colLines = append(colLines, lines)
		if len(lines) > maxHeight {
			maxHeight = len(lines)
		}
	}

	emptyCell := strings.Repeat(" ", colWidth)
	rows := []string{pad + headerSB.String(), ruleRow}
	if errLine := keysErrorLine(m.width); errLine != "" {
		rows = append([]string{errLine}, rows...)
	}
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

func (m SlotsModel) renderSessionLines(slotIdx int, colWidth int) []string {
	sessions := m.slots[slotIdx]
	isCursorCol := slotIdx == m.colSlot

	plain := lipgloss.NewStyle().Width(colWidth)
	cursor := lipgloss.NewStyle().Width(colWidth).Background(lipgloss.Color("237"))

	if len(sessions) == 0 {
		var s lipgloss.Style
		if isCursorCol {
			s = cursor.Foreground(lipgloss.Color("243"))
		} else {
			s = plain.Foreground(lipgloss.Color("243"))
		}
		return []string{s.Render("-")}
	}

	const cutLabel = " (cut)"
	const cutLabelWidth = len(cutLabel)

	var lines []string
	for ri, sess := range sessions {
		var s lipgloss.Style
		if isCursorCol && ri == m.colRow {
			s = cursor
		} else {
			s = plain
		}
		var cell string
		if sess.ID == m.cutSessID {
			nameWidth := colWidth - cutLabelWidth
			if nameWidth < 1 {
				nameWidth = 1
			}
			cell = s.Render(truncate(sess.Name, nameWidth) + dimStyle.Render(cutLabel))
		} else {
			cell = s.Render(truncate(sess.Name, colWidth))
		}
		lines = append(lines, cell)
	}
	return lines
}

func runSlotsBody(args []string) {
	fs := flag.NewFlagSet("show-sessions-body", flag.ExitOnError)
	commandFile := fs.String("command-file", "", "write command here")
	returnView := fs.String("return-view", "", "view name to reopen after add-session")
	fs.Parse(args)

	initialSessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hometown: %v\n", err)
		os.Exit(1)
	}

	activationKey := tmuxGetGlobalOption("@hometown_activation_key")
	if activationKey == "" {
		activationKey = "u"
	}

	m, err := newSlotsModel(initialSessID, *commandFile, *returnView, activationKey, shiftOf(activationKey), getCyclePattern())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hometown: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "hometown: %v\n", err)
		os.Exit(1)
	}
}
