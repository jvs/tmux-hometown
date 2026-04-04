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

type StoresModel struct {
	stores map[string][]Session

	// Column cursor
	colLane int // 0–4, index into storeKeys
	colRow  int // index into stores[storeKeys[colLane]]

	// Cut/paste
	cutSessID string

	// Input mode (add / rename)
	inputMode   bool
	inputPrompt string
	inputValue  []rune
	modalAction ModalAction

	// Confirm modal
	modal tea.Model

	// Assign-key prompt (shown when current session has no store key)
	promptMode bool

	// Command file for deferred operations
	commandFile   string
	returnView    string
	switchView    string

	// Session to restore on cancel
	initialSessID   string
	initialSessName string

	width  int
	height int
}

func newStoresModel(initialSessID, commandFile, returnView, switchView string) (StoresModel, error) {
	promptMode := getSessionStoreKey(initialSessID) == "" &&
		tmuxGetSessionOption(initialSessID, "@hometown_store_never") != "1"

	sess, _ := loadSession(initialSessID)

	m := StoresModel{
		stores:          groupByStore(),
		promptMode:      promptMode,
		commandFile:     commandFile,
		returnView:      returnView,
		switchView:      switchView,
		initialSessID:   initialSessID,
		initialSessName: sess.Name,
		width:           80,
		height:          24,
	}
	m.positionOnSession(initialSessID)
	return m, nil
}

func (m *StoresModel) positionOnSession(sessID string) {
	for li, key := range storeKeys {
		for ri, s := range m.stores[key] {
			if s.ID == sessID {
				m.colLane = li
				m.colRow = ri
				return
			}
		}
	}
}

func (m StoresModel) Init() tea.Cmd { return nil }

func (m StoresModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m StoresModel) handleInputKey(msg tea.KeyMsg) (StoresModel, tea.Cmd) {
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

func (m StoresModel) handlePromptKey(msg tea.KeyMsg) (StoresModel, tea.Cmd) {
	switch msg.String() {
	case "s":
		m.promptMode = false
		return m, nil
	case "n":
		tmuxSetSessionOption(m.initialSessID, "@hometown_store_never", "1")
		m.promptMode = false
		return m, nil
	case "esc", "alt+U":
		if m.initialSessID != "" {
			tmuxRun("switch-client", "-t", m.initialSessID)
		}
		return m, tea.Quit
	case "alt+u", "u", "U", "shift+enter":
		if m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-windows\n"), 0644)
		}
		return m, tea.Quit
	}
	// h/j/k/l/; assign the selected key to the current session.
	for _, key := range storeKeys {
		if msg.String() == laneKeyToUserKey(key) || msg.String() == key {
			setSessionStoreKey(m.initialSessID, key)
			m.stores = groupByStore()
			m.positionOnSession(m.initialSessID)
			m.promptMode = false
			return m, nil
		}
	}
	return m, nil
}

func (m StoresModel) handleKey(msg tea.KeyMsg) (StoresModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m, tea.Quit

	case "esc", "alt+U":
		if m.initialSessID != "" {
			tmuxRun("switch-client", "-t", m.initialSessID)
		}
		return m, tea.Quit

	case "alt+u", "u", "U", "shift+enter":
		if m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-windows\n"), 0644)
		}
		return m, tea.Quit

	case "alt+o":
		if m.switchView != "" && m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-"+m.switchView+"\n"), 0644)
		}
		return m, tea.Quit

	case "a":
		m.inputMode = true
		m.inputPrompt = "Name"
		m.inputValue = nil
		m.modalAction = ActionAdd
		return m, nil

	case "r":
		if s := m.currentSession(); s != nil {
			m.inputMode = true
			m.inputPrompt = "Rename"
			m.inputValue = []rune(s.Name)
			m.modalAction = ActionRename
		}
		return m, nil

	case "d":
		if s := m.currentSession(); s != nil {
			m.modal = newConfirmModal(fmt.Sprintf("Kill session %q?", s.Name))
			m.modalAction = ActionDelete
		}
		return m, nil

	case "m":
		if s := m.currentSession(); s != nil {
			clearStoreForSession(s.ID)
			m.refresh()
		}
		return m, nil

	case "c", "x":
		if s := m.currentSession(); s != nil {
			m.cutSessID = s.ID
		}
		return m, nil

	case "p", "P":
		return m.handlePaste()

	case "down":
		sessions := m.stores[storeKeys[m.colLane]]
		if m.colRow < len(sessions)-1 {
			m.colRow++
		}
		return m, m.switchToCurrentCmd()

	case "up":
		if m.colRow > 0 {
			m.colRow--
		}
		return m, m.switchToCurrentCmd()

	case "right":
		if m.colLane < len(storeKeys)-1 {
			m.colLane++
			m.clampColRow()
		}
		return m, m.switchToCurrentCmd()

	case "left":
		if m.colLane > 0 {
			m.colLane--
			m.clampColRow()
		}
		return m, m.switchToCurrentCmd()
	}

	// Shift+key: switch to that store and show its lanes.
	if laneIdx, ok := laneKeyLane[msg.String()]; ok && laneKeyShift[msg.String()] {
		storeKey := storeKeys[laneIdx]
		if m.commandFile != "" {
			exe, _ := os.Executable()
			content := exe + " switch-session-and-show-lanes " + storeKey + "\n"
			os.WriteFile(m.commandFile, []byte(content), 0644)
		}
		return m, tea.Quit
	}

	// Store keys: h/j/k/l/; jump to that column (or cycle within if already there).
	if laneIdx, ok := laneKeyLane[msg.String()]; ok {
		if m.colLane == laneIdx {
			sessions := m.stores[storeKeys[laneIdx]]
			if n := len(sessions); n > 0 {
				m.colRow = (m.colRow + 1) % n
			}
		} else {
			m.colLane = laneIdx
			sessions := m.stores[storeKeys[laneIdx]]
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

func (m *StoresModel) clampColRow() {
	sessions := m.stores[storeKeys[m.colLane]]
	if len(sessions) > 0 && m.colRow >= len(sessions) {
		m.colRow = len(sessions) - 1
	}
	if len(sessions) == 0 {
		m.colRow = 0
	}
}

func (m StoresModel) handleModalDone(msg ModalDoneMsg) (StoresModel, tea.Cmd) {
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

func (m StoresModel) handleAdd(name string) (StoresModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	key := storeKeys[m.colLane]

	if m.commandFile != "" {
		exe, _ := os.Executable()
		content := fmt.Sprintf(
			"NEWSESS=$(tmux new-session -d -s '%s' -P -F '#{session_id}' 2>/dev/null || tmux new-session -d -P -F '#{session_id}')\n"+
				"tmux set-option -t \"$NEWSESS\" @hometown_store_key %s\n",
			name, key)
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
	setSessionStoreKey(newSessID, key)
	m.refresh()
	return m, nil
}

func (m StoresModel) handleRename(name string) (StoresModel, tea.Cmd) {
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

func (m StoresModel) handleDelete() (StoresModel, tea.Cmd) {
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

func (m StoresModel) findFallbackSession(deletedID string, all []Session) string {
	// 1. Another session in the same column.
	for _, s := range m.stores[storeKeys[m.colLane]] {
		if s.ID != deletedID {
			return s.ID
		}
	}

	// 2. Any session with a key assigned.
	for _, key := range storeKeys {
		for _, s := range m.stores[key] {
			if s.ID != deletedID {
				return s.ID
			}
		}
	}

	// 3. Previously-viewed session.
	out, err := exec.Command("tmux", "display-message", "-p", "#{client_last_session}").Output()
	if err == nil {
		prevName := strings.TrimSpace(string(out))
		for _, s := range all {
			if s.ID != deletedID && s.Name == prevName {
				return s.ID
			}
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

func (m StoresModel) handlePaste() (StoresModel, tea.Cmd) {
	if m.cutSessID == "" {
		return m, nil
	}
	key := storeKeys[m.colLane]
	setSessionStoreKey(m.cutSessID, key)
	pastedID := m.cutSessID
	m.cutSessID = ""
	m.refresh()
	m.positionOnSession(pastedID)
	return m, nil
}

func (m StoresModel) currentSession() *Session {
	sessions := m.stores[storeKeys[m.colLane]]
	if m.colRow >= 0 && m.colRow < len(sessions) {
		s := sessions[m.colRow]
		return &s
	}
	return nil
}

func (m StoresModel) switchToCurrentCmd() tea.Cmd {
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

func (m *StoresModel) refresh() {
	m.stores = groupByStore()
	m.clampColRow()
	if m.cutSessID != "" {
		found := false
		for _, sessions := range m.stores {
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

func (m StoresModel) jColumnOffset() int {
	const sidePad = 2
	const gaps = 4 * 2
	colWidth := max(10, (m.width-2*sidePad-gaps)/5)
	return sidePad + colWidth + 2
}

func (m StoresModel) View() string {
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
			hintStyle.Render("[a]dd   [r]ename   [d]elete   [c]ut   [p]aste   re[m]ove"))
	}

	content := m.viewGrid()
	padding := m.height - (strings.Count(content, "\n") + 1)
	if padding < 1 {
		padding = 1
	}
	return content + strings.Repeat("\n", padding) + bar
}

func (m StoresModel) viewPrompt() string {
	question := lipgloss.NewStyle().Render(
		fmt.Sprintf("Assign a key to session %q?", m.initialSessName))
	options := hintStyle.Render("[H] [J] [K] [L] [;]  [s]kip  [n]ever")
	line := question + "  " + options
	centered := lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(line)
	top := strings.Repeat("\n", m.height/2-1)
	return top + centered
}

func (m StoresModel) viewGrid() string {
	const sidePad = 2
	const gaps = 4 * 2
	colWidth := max(10, (m.width-2*sidePad-gaps)/5)
	contentWidth := 5*colWidth + gaps
	pad := strings.Repeat(" ", sidePad)

	var headerSB strings.Builder
	for li, key := range storeKeys {
		var s lipgloss.Style
		if li == m.colLane {
			s = lipgloss.NewStyle().Width(colWidth).Bold(true)
		} else {
			s = lipgloss.NewStyle().Width(colWidth).Foreground(lipgloss.Color("246"))
		}
		headerSB.WriteString(s.Render(storeDisplayNames[key]))
		if li < len(storeKeys)-1 {
			headerSB.WriteString("  ")
		}
	}

	ruleRow := pad + guideStyle.Render(strings.Repeat("─", contentWidth))

	var colLines [][]string
	maxHeight := 0
	for li, key := range storeKeys {
		lines := m.renderSessionLines(li, key, colWidth)
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

func (m StoresModel) renderSessionLines(laneIdx int, key string, colWidth int) []string {
	sessions := m.stores[key]
	isCursorCol := laneIdx == m.colLane

	plain := lipgloss.NewStyle().Width(colWidth)
	cursor := lipgloss.NewStyle().Width(colWidth).Background(lipgloss.Color("237"))

	if len(sessions) == 0 {
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

func runStoresBody(args []string) {
	fs := flag.NewFlagSet("show-sessions-body", flag.ExitOnError)
	commandFile := fs.String("command-file", "", "write command here")
	returnView := fs.String("return-view", "", "view name to reopen after add-session")
	switchView := fs.String("switch-view", "", "view name to switch to via alt+o")
	fs.Parse(args)

	initialSessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hometown: %v\n", err)
		os.Exit(1)
	}

	m, err := newStoresModel(initialSessID, *commandFile, *returnView, *switchView)
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
