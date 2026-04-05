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

// ── Column indices ────────────────────────────────────────────────────────────

// allColSession is the column index for the session name column in the grid.
// Window lane columns are 1..len(laneOrder).
const allColSession = 0

// Fixed widths for the key and session columns.
const (
	allKeyColW  = 3
	allSessColW = 16
	allSidePad  = 2
)

var (
	allHeaderRuleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
)

// ── Data types ────────────────────────────────────────────────────────────────

// allRow holds one row of the show-grid table: one slot and the
// first window in each lane for the primary session assigned to that slot.
type allRow struct {
	slotKey int
	sess    *Session       // nil when the slot is empty
	windows map[int]Window // lane index → first window in that lane
}

// AllModel is the TUI model for show-grid.
type AllModel struct {
	rows []allRow

	curRow int // 0–4, index into slotKeys
	curCol int // 0=Session col, 1–5=laneOrder[curCol-1]

	cutWinID  string
	cutSessID string

	inputMode   bool
	inputPrompt string
	inputValue  []rune
	modalAction ModalAction

	modal tea.Model

	commandFile        string
	initialSessID      string
	initialWinID       string
	activationKey      string
	shiftActivationKey string
	cyclePattern       string

	width  int
	height int
}

// ── Loading ───────────────────────────────────────────────────────────────────

func loadAllRows() []allRow {
	slots := groupBySlot()
	rows := make([]allRow, len(slotKeys))
	for i := range slotKeys {
		row := allRow{
			slotKey: i,
			windows: make(map[int]Window),
		}
		sessions := slots[i]
		if len(sessions) > 0 {
			s := sessions[0]
			row.sess = &s
			wins, _ := loadWindows(s.ID)
			for _, w := range wins {
				if w.Lane >= 0 {
					if _, exists := row.windows[w.Lane]; !exists {
						row.windows[w.Lane] = w
					}
				}
			}
		}
		rows[i] = row
	}
	return rows
}

func newAllModel(initialSessID, initialWinID, commandFile, activationKey, shiftActivationKey, cyclePattern string) AllModel {
	m := AllModel{
		rows:               loadAllRows(),
		commandFile:        commandFile,
		activationKey:      activationKey,
		shiftActivationKey: shiftActivationKey,
		cyclePattern:       cyclePattern,
		initialSessID:      initialSessID,
		initialWinID:       initialWinID,
		width:              89,
		height:             18,
	}
	for i, row := range m.rows {
		if row.sess != nil && row.sess.ID == initialSessID {
			m.curRow = i
			m.curCol = getCurrentLane() + 1
			break
		}
	}
	return m
}

// ── Bubbletea interface ───────────────────────────────────────────────────────

func (m AllModel) Init() tea.Cmd { return nil }

func (m AllModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m AllModel) handleInputKey(msg tea.KeyMsg) (AllModel, tea.Cmd) {
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

func (m AllModel) handleKey(msg tea.KeyMsg) (AllModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.curCol != allColSession && m.rows[m.curRow].sess == nil {
			return m.handleEnterEmptySessionWindow()
		}
		return m, tea.Quit

	case "esc":
		tmuxRun("switch-client", "-t", m.initialSessID+":"+m.initialWinID)
		return m, tea.Quit

	case "up", "k":
		if m.curRow > 0 {
			m.curRow--
		}
		return m, m.switchToCurrentCmd()

	case "down", "j":
		if m.curRow < len(m.rows)-1 {
			m.curRow++
		}
		return m, m.switchToCurrentCmd()

	case "left", "h":
		if m.curCol > 0 {
			m.curCol--
		}
		return m, m.switchToCurrentCmd()

	case "right", "l":
		if m.curCol < len(laneOrder) {
			m.curCol++
		}
		return m, m.switchToCurrentCmd()

	case "a":
		return m.startAdd()

	case "r":
		return m.startRename()

	case "d":
		if m.curCol == allColSession {
			if s := m.currentSess(); s != nil {
				m.modal = newConfirmModal(fmt.Sprintf("Kill session %q?", s.Name))
				m.modalAction = ActionDelete
			}
		} else {
			if w := m.currentWin(); w != nil {
				m.modal = newConfirmModal(fmt.Sprintf("Kill window %q?", w.Name))
				m.modalAction = ActionDelete
			}
		}
		return m, nil

	case "m":
		return m.handleRemove()

	case "c", "x":
		return m.handleCut()

	case "p":
		return m.handlePaste()
	}

	// Activation key / tab: cycle through popups.
	key := msg.String()
	if key == m.activationKey || key == "tab" {
		return m, cyclePopup("grid", m.cyclePattern, m.commandFile, true)
	}
	if key == m.shiftActivationKey || key == "shift+tab" {
		return m, cyclePopup("grid", m.cyclePattern, m.commandFile, false)
	}

	// alt+hjkl/; — jump to that window column.
	if laneIdx, ok := altLaneKeyLane[msg.String()]; ok {
		m.curCol = laneIdx + 1
		return m, m.switchToCurrentCmd()
	}

	// alt+shift+hjkl/: — jump to that slot row.
	if laneIdx, ok := altShiftLaneKeyLane[msg.String()]; ok {
		m.curRow = laneIdx
		return m, m.switchToCurrentCmd()
	}

	return m, nil
}

// ── Actions ───────────────────────────────────────────────────────────────────

func (m AllModel) handleModalDone(msg ModalDoneMsg) (AllModel, tea.Cmd) {
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

func (m AllModel) startAdd() (AllModel, tea.Cmd) {
	if m.curCol == allColSession {
		m.inputMode = true
		m.inputPrompt = "Session name"
		m.inputValue = nil
		m.modalAction = ActionAdd
	} else if m.rows[m.curRow].sess != nil {
		m.inputMode = true
		m.inputPrompt = "Window name"
		m.inputValue = nil
		m.modalAction = ActionAdd
	}
	return m, nil
}

func (m AllModel) startRename() (AllModel, tea.Cmd) {
	if m.curCol == allColSession {
		if s := m.currentSess(); s != nil {
			m.inputMode = true
			m.inputPrompt = "Rename session"
			m.inputValue = []rune(s.Name)
			m.modalAction = ActionRename
		}
	} else {
		if w := m.currentWin(); w != nil {
			m.inputMode = true
			m.inputPrompt = "Rename window"
			m.inputValue = []rune(w.Name)
			m.modalAction = ActionRename
		}
	}
	return m, nil
}

func (m AllModel) handleAdd(name string) (AllModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	if m.curCol == allColSession {
		return m.handleAddSession(name)
	}
	return m.handleAddWindow(name)
}

func (m AllModel) handleAddSession(name string) (AllModel, tea.Cmd) {
	key := m.curRow // slot index
	if m.commandFile != "" {
		exe, _ := os.Executable()
		content := fmt.Sprintf(
			"NEWSESS=$(tmux new-session -d -s %s -P -F '#{session_id}' 2>/dev/null || tmux new-session -d -P -F '#{session_id}')\n"+
				"tmux set-option -t \"$NEWSESS\" @hometown_slot %s\n"+
				"NEWWIN=$(tmux display-message -t \"$NEWSESS\" -p '#{window_id}')\n"+
				"%s record-window-visit \"$NEWWIN\"\n"+
				"%s show-grid\n",
			shellSingleQuote(name), storeIndex(key), exe, exe)
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

func (m AllModel) handleAddWindow(name string) (AllModel, tea.Cmd) {
	row := m.rows[m.curRow]
	if row.sess == nil {
		return m, nil
	}
	laneKey := m.curCol - 1 // lane index
	wins, _ := loadWindows(row.sess.ID)

	// Insert after the last window in the target lane, or after the last
	// window overall, or at the beginning of an empty session.
	var targetID string
	position := "a"
	for _, w := range wins {
		if w.Lane == laneKey {
			targetID = w.ID
		}
	}
	if targetID == "" && len(wins) > 0 {
		targetID = wins[len(wins)-1].ID
	}
	if targetID == "" {
		targetID = row.sess.ID
		position = "b"
	}

	if m.commandFile != "" {
		exe, _ := os.Executable()
		content := fmt.Sprintf(
			"NEWWIN=$(tmux new-window -%s -t '%s' -n %s -c '#{pane_current_path}' -P -F '#{window_id}')\n"+
				"tmux set-window-option -t \"$NEWWIN\" @hometown_lane %s\n"+
				"%s record-window-visit \"$NEWWIN\"\n"+
				"%s show-grid\n",
			position, targetID, shellSingleQuote(name), storeIndex(laneKey), exe, exe)
		os.WriteFile(m.commandFile, []byte(content), 0644)
		return m, tea.Quit
	}

	out, err := exec.Command("tmux", "new-window",
		"-"+position, "-t", targetID,
		"-n", name, "-c", "#{pane_current_path}",
		"-P", "-F", "#{window_id}").Output()
	if err == nil {
		newWinID := strings.TrimSpace(string(out))
		tmuxRun("set-window-option", "-t", newWinID, "@hometown_lane", storeIndex(laneKey))
		recordWindowVisit(newWinID)
	}
	m.refresh()
	return m, nil
}

func (m AllModel) handleEnterEmptySessionWindow() (AllModel, tea.Cmd) {
	slotKey := m.rows[m.curRow].slotKey
	laneKey := m.curCol - 1
	sessName := "Session " + indexName(slotKey)
	winName := "Window " + indexName(laneKey)

	if m.commandFile != "" {
		exe, _ := os.Executable()
		content := fmt.Sprintf(
			"NEWSESS=$(tmux new-session -d -s %s -P -F '#{session_id}' 2>/dev/null || tmux new-session -d -P -F '#{session_id}')\n"+
				"tmux set-option -t \"$NEWSESS\" @hometown_slot %s\n"+
				"NEWWIN=$(tmux display-message -t \"$NEWSESS\" -p '#{window_id}')\n"+
				"tmux rename-window -t \"$NEWWIN\" %s\n"+
				"tmux set-window-option -t \"$NEWWIN\" @hometown_lane %s\n"+
				"%s record-window-visit \"$NEWWIN\"\n"+
				"tmux switch-client -t \"$NEWSESS\"\n",
			shellSingleQuote(sessName), storeIndex(slotKey), shellSingleQuote(winName), storeIndex(laneKey), exe)
		os.WriteFile(m.commandFile, []byte(content), 0644)
		return m, tea.Quit
	}

	out, err := exec.Command("tmux", "new-session", "-d", "-s", sessName, "-P", "-F", "#{session_id}").Output()
	if err != nil {
		out, err = exec.Command("tmux", "new-session", "-d", "-P", "-F", "#{session_id}").Output()
		if err != nil {
			return m, nil
		}
	}
	newSessID := strings.TrimSpace(string(out))
	setSessionSlotKey(newSessID, slotKey)
	winOut, _ := exec.Command("tmux", "list-windows", "-t", newSessID, "-F", "#{window_id}").Output()
	if winID := strings.TrimSpace(string(winOut)); winID != "" {
		tmuxRun("rename-window", "-t", winID, winName)
		tmuxRun("set-window-option", "-t", winID, "@hometown_lane", storeIndex(laneKey))
		recordWindowVisit(winID)
	}
	tmuxRun("switch-client", "-t", newSessID)
	return m, tea.Quit
}

func (m AllModel) handleRename(name string) (AllModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	if m.curCol == allColSession {
		if s := m.currentSess(); s != nil {
			tmuxRun("rename-session", "-t", s.ID, name)
		}
	} else {
		if w := m.currentWin(); w != nil {
			tmuxRun("rename-window", "-t", w.ID, name)
		}
	}
	m.refresh()
	return m, nil
}

func (m AllModel) handleDelete() (AllModel, tea.Cmd) {
	if m.curCol == allColSession {
		return m.handleDeleteSession()
	}
	return m.handleDeleteWindow()
}

func (m AllModel) handleDeleteSession() (AllModel, tea.Cmd) {
	s := m.currentSess()
	if s == nil {
		return m, nil
	}
	all, _ := listAllSessions()
	if len(all) <= 1 {
		tmuxRun("kill-session", "-t", s.ID)
		return m, tea.Quit
	}
	fallbackTarget := findFallbackTarget(s.ID, all)
	tmuxRun("switch-client", "-t", fallbackTarget)
	tmuxRun("kill-session", "-t", s.ID)
	m.refresh()
	return m, nil
}

func (m AllModel) handleDeleteWindow() (AllModel, tea.Cmd) {
	row := m.rows[m.curRow]
	if row.sess == nil {
		return m, nil
	}
	w := m.currentWin()
	if w == nil {
		return m, nil
	}
	winID := w.ID
	wins, _ := loadWindows(row.sess.ID)

	// Switch to another window in the session before killing.
	for _, other := range wins {
		if other.ID != winID {
			tmuxRun("switch-client", "-t", row.sess.ID+":"+other.ID)
			tmuxRun("kill-window", "-t", winID)
			m.refresh()
			return m, nil
		}
	}

	// Last window — killing it kills the session.
	return m.handleDeleteSession()
}

func (m AllModel) handleRemove() (AllModel, tea.Cmd) {
	if m.curCol == allColSession {
		if s := m.currentSess(); s != nil {
			clearSlotForSession(s.ID)
		}
	} else {
		if w := m.currentWin(); w != nil {
			exec.Command("tmux", "set-window-option", "-u", "-t", w.ID, "@hometown_lane").Run()
		}
	}
	m.refresh()
	return m, nil
}

func (m AllModel) handleCut() (AllModel, tea.Cmd) {
	if m.curCol == allColSession {
		if s := m.currentSess(); s != nil {
			m.cutSessID = s.ID
			m.cutWinID = ""
		}
	} else {
		if w := m.currentWin(); w != nil {
			m.cutWinID = w.ID
			m.cutSessID = ""
		}
	}
	return m, nil
}

func (m AllModel) handlePaste() (AllModel, tea.Cmd) {
	if m.cutSessID != "" && m.curCol == allColSession {
		setSessionSlotKey(m.cutSessID, m.curRow)
		m.cutSessID = ""
		m.refresh()
	} else if m.cutWinID != "" && m.curCol != allColSession {
		laneKey := m.curCol - 1
		tmuxRun("set-window-option", "-t", m.cutWinID, "@hometown_lane", storeIndex(laneKey))
		m.cutWinID = ""
		m.refresh()
	}
	return m, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m AllModel) currentSess() *Session {
	if m.curRow < len(m.rows) {
		return m.rows[m.curRow].sess
	}
	return nil
}

func (m AllModel) currentWin() *Window {
	if m.curCol == allColSession || m.curRow >= len(m.rows) {
		return nil
	}
	row := m.rows[m.curRow]
	if w, ok := row.windows[m.curCol-1]; ok {
		wCopy := w
		return &wCopy
	}
	return nil
}

func (m AllModel) switchToCurrentCmd() tea.Cmd {
	row := m.rows[m.curRow]
	if row.sess == nil {
		return nil
	}
	if m.curCol == allColSession {
		sessID := row.sess.ID
		return func() tea.Msg {
			tmuxRun("switch-client", "-t", sessID)
			return nil
		}
	}
	w := m.currentWin()
	if w == nil {
		return nil
	}
	target := row.sess.ID + ":" + w.ID
	return func() tea.Msg {
		tmuxRun("switch-client", "-t", target)
		return nil
	}
}

func (m *AllModel) refresh() {
	m.rows = loadAllRows()
	if m.curRow >= len(m.rows) {
		m.curRow = len(m.rows) - 1
	}
}

// winColWidth returns the width of each window lane column.
func (m AllModel) winColWidth() int {
	const gaps = 4 * 2 // 4 inter-column gaps of 2 spaces each
	available := m.width - 2*allSidePad - allKeyColW - 1 - allSessColW - 2 - gaps
	return max(6, available/5)
}

// totalWidth returns the full content width (excluding side padding).
func (m AllModel) totalWidth() int {
	w := m.winColWidth()
	return allKeyColW + 1 + allSessColW + 2 + 5*w + 4*2
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m AllModel) View() string {
	pad := strings.Repeat(" ", allSidePad)
	tw := m.totalWidth()

	var lines []string
	lines = append(lines, "") // blank line at top
	if errLine := keysErrorLine(m.width); errLine != "" {
		lines = append(lines, errLine)
	}
	lines = append(lines, pad+m.renderHeader())
	lines = append(lines, pad+allHeaderRuleStyle.Render(strings.Repeat("─", tw)))
	for i, row := range m.rows {
		lines = append(lines, pad+m.renderRow(i, row))
	}

	content := strings.Join(lines, "\n")
	bar := m.renderBar()
	padding := m.height - strings.Count(content, "\n") - strings.Count(bar, "\n") - 1
	if padding < 1 {
		padding = 1
	}
	return content + strings.Repeat("\n", padding) + bar
}

func (m AllModel) renderHeader() string {
	w := m.winColWidth()
	keyHead := lipgloss.NewStyle().Width(allKeyColW).Render("")
	sessHead := lipgloss.NewStyle().Width(allSessColW).Render("Session")

	var sb strings.Builder
	sb.WriteString(keyHead)
	sb.WriteString(" ")
	sb.WriteString(sessHead)
	sb.WriteString("  ")
	for i := range laneOrder {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(lipgloss.NewStyle().Width(w).Render(indexDisplay(i)))
	}
	return sb.String()
}

func (m AllModel) renderRow(rowIdx int, row allRow) string {
	w := m.winColWidth()
	isCurRow := rowIdx == m.curRow

	// Non-current rows use a darker foreground throughout.
	var fg lipgloss.Color
	if isCurRow {
		fg = lipgloss.Color("")
	} else {
		fg = lipgloss.Color("243")
	}
	emptyFg := lipgloss.Color("240")
	if isCurRow {
		emptyFg = lipgloss.Color("243")
	}

	// Key cell.
	keyCell := lipgloss.NewStyle().Width(allKeyColW).
		Foreground(fg).
		Render(indexDisplay(row.slotKey))

	// Session cell.
	inSessCur := isCurRow && m.curCol == allColSession
	sessBase := lipgloss.NewStyle().Width(allSessColW).Foreground(fg)
	if inSessCur {
		sessBase = sessBase.Background(lipgloss.Color("237"))
	}
	var sessCell string
	if row.sess == nil {
		sessCell = sessBase.Foreground(emptyFg).Render("-")
	} else {
		name := row.sess.Name
		if row.sess.ID == m.cutSessID {
			const cutLabel = " (cut)"
			name = truncate(name, allSessColW-len(cutLabel)) + dimStyle.Render(cutLabel)
		} else {
			name = truncate(name, allSessColW)
		}
		sessCell = sessBase.Render(name)
	}

	var sb strings.Builder
	sb.WriteString(keyCell)
	sb.WriteString(" ")
	sb.WriteString(sessCell)
	sb.WriteString("  ")

	// Window cells.
	if row.sess == nil {
		for i := 0; i < len(laneOrder); i++ {
			if i > 0 {
				sb.WriteString("  ")
			}
			colIdx := i + 1
			inWinCur := isCurRow && m.curCol == colIdx
			dotStyle := lipgloss.NewStyle().Width(w).Foreground(emptyFg)
			if inWinCur {
				dotStyle = dotStyle.Background(lipgloss.Color("237"))
			}
			sb.WriteString(dotStyle.Render("-"))
		}
	} else {
		for i := range laneOrder {
			if i > 0 {
				sb.WriteString("  ")
			}
			colIdx := i + 1
			inWinCur := isCurRow && m.curCol == colIdx
			winBase := lipgloss.NewStyle().Width(w).Foreground(fg)
			if inWinCur {
				winBase = winBase.Background(lipgloss.Color("237"))
			}

			win, hasWin := row.windows[i]
			if !hasWin {
				sb.WriteString(winBase.Foreground(emptyFg).Render("-"))
				continue
			}
			if win.ID == m.cutWinID {
				const cutLabel = " (cut)"
				nameW := w - len(cutLabel)
				if nameW < 1 {
					nameW = 1
				}
				sb.WriteString(winBase.Render(truncate(win.Name, nameW) + dimStyle.Render(cutLabel)))
			} else {
				sb.WriteString(winBase.Render(truncate(win.Name, w)))
			}
		}
	}
	return sb.String()
}

func (m AllModel) renderBar() string {
	pad := strings.Repeat(" ", allSidePad)
	if m.modal != nil {
		return pad + m.modal.View()
	}
	if m.inputMode {
		return pad + dimStyle.Render(m.inputPrompt+": ") + string(m.inputValue) + cursorStyle.Render("█")
	}
	return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(
		hintStyle.Render("[a]dd   [r]ename   [d]elete   [c]ut   [p]aste   re[m]ove"))
}

// ── Entry point ───────────────────────────────────────────────────────────────

func runGridBody(args []string) {
	fs := flag.NewFlagSet("show-grid-body", flag.ExitOnError)
	commandFile := fs.String("command-file", "", "write deferred commands here")
	fs.Parse(args)

	sessID, winID, err := getCurrentSessionAndWindow()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hometown: %v\n", err)
		os.Exit(1)
	}

	activationKey := tmuxGetGlobalOption("@hometown_activation_key")
	if activationKey == "" {
		activationKey = "u"
	}

	m := newAllModel(sessID, winID, *commandFile, activationKey, shiftOf(activationKey), getCyclePattern())
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "hometown: %v\n", err)
		os.Exit(1)
	}
}
