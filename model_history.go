package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	histSessColW = 22
	histWinColW  = 30
	histTimeColW = 10
	histRightPad = 1
	// histRowW is the total width of one rendered row (excluding histPad).
	histRowW  = histSessColW + 2 + histWinColW + 2 + histTimeColW + histRightPad
	histRuleW = histRowW
	histPad   = "  "
)

var (
	histHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	histRuleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	histCursorBg    = lipgloss.NewStyle().Background(lipgloss.Color("237"))
)

// historyEntry holds the display data for one row of the history popup.
type historyEntry struct {
	SessionID   string
	SessionName string
	WindowID    string
	WindowName  string
	Visited     int64 // nanoseconds
}

// HistoryModel is the TUI model for show-history.
type HistoryModel struct {
	entries            []historyEntry
	cursor             int
	offset             int
	width              int
	height             int
	commandFile        string
	activationKey      string
	shiftActivationKey string
	cyclePattern       string
	initialSessID      string
	initialWinID       string
}

func newHistoryModel(commandFile, activationKey, shiftActivationKey, cyclePattern string) (HistoryModel, error) {
	entries, err := buildHistoryEntries()
	if err != nil {
		return HistoryModel{}, err
	}

	initialSessID, initialWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return HistoryModel{}, err
	}

	return HistoryModel{
		entries:            entries,
		width:              71,
		height:             28,
		commandFile:        commandFile,
		activationKey:      activationKey,
		shiftActivationKey: shiftActivationKey,
		cyclePattern:       cyclePattern,
		initialSessID:      initialSessID,
		initialWinID:       initialWinID,
	}, nil
}

// buildHistoryEntries loads all visited windows, attaches session/window names,
// and returns them sorted most-recent-first.
func buildHistoryEntries() ([]historyEntry, error) {
	visits, err := listAllWindowVisits()
	if err != nil {
		return nil, err
	}
	if len(visits) == 0 {
		return nil, nil
	}

	// Fetch session and window names in one call. Window name is last so
	// SplitN handles names that contain spaces.
	out, _ := exec.Command("tmux", "list-windows", "-a", "-F",
		"#{session_id}\t#{window_id}\t#{session_name}\t#{window_name}").Output()

	type nameInfo struct{ sessName, winName string }
	names := map[string]nameInfo{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		names[parts[0]+":"+parts[1]] = nameInfo{parts[2], parts[3]}
	}

	entries := make([]historyEntry, 0, len(visits))
	for _, v := range visits {
		n := names[v.SessionID+":"+v.WindowID]
		entries = append(entries, historyEntry{
			SessionID:   v.SessionID,
			SessionName: n.sessName,
			WindowID:    v.WindowID,
			WindowName:  n.winName,
			Visited:     v.Visited,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Visited > entries[j].Visited
	})
	return entries, nil
}

// formatRelativeAge converts a nanosecond Unix timestamp to a human-readable
// relative age, always using relative units regardless of how old.
func formatRelativeAge(ns int64) string {
	ago := time.Since(time.Unix(0, ns))
	switch {
	case ago < time.Minute:
		return fmt.Sprintf("%ds ago", int(ago.Seconds()))
	case ago < time.Hour:
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	case ago < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(ago.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(ago.Hours()/24))
	}
}

// calcHistoryHeight computes a snug popup height for the current number of
// visited windows, capped at the same maximum as show-state (28 lines).
// The +7 matches the pattern used by calcLanesHeight and calcSlotsHeight.
func calcHistoryHeight() int {
	visits, err := listAllWindowVisits()
	n := 0
	if err == nil {
		n = len(visits)
	}
	h := n + 7
	if h > 28 {
		h = 28
	}
	if h < 8 {
		h = 8
	}
	if keysError != "" {
		h++
	}
	return h
}

func (m HistoryModel) Init() tea.Cmd { return nil }

func (m HistoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// previewCurrentCmd returns a Cmd that switches the tmux client to the
// currently highlighted entry, showing it behind the popup.
func (m HistoryModel) previewCurrentCmd() tea.Cmd {
	if len(m.entries) == 0 {
		return nil
	}
	e := m.entries[m.cursor]
	v := &visitedWindow{SessionID: e.SessionID, WindowID: e.WindowID}
	return func() tea.Msg {
		switchToWindow(v)
		return nil
	}
}

func (m HistoryModel) handleKey(msg tea.KeyMsg) (HistoryModel, tea.Cmd) {
	n := len(m.entries)
	visible := m.visibleLines()

	switch msg.String() {
	case "q", "esc":
		// Restore the window that was active when the popup opened.
		if m.initialSessID != "" {
			tmuxRun("switch-client", "-t", m.initialSessID+":"+m.initialWinID)
		}
		return m, tea.Quit

	case "enter":
		// The window is already focused via the last preview; just record
		// the visit and close.
		if n > 0 {
			recordWindowVisit(m.entries[m.cursor].WindowID)
		}
		return m, tea.Quit

	case "j", "down":
		if m.cursor < n-1 {
			m.cursor++
			if m.cursor >= m.offset+visible {
				m.offset++
			}
		}
		return m, m.previewCurrentCmd()

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset--
			}
		}
		return m, m.previewCurrentCmd()

	case "ctrl+d":
		m.cursor += visible / 2
		if m.cursor >= n {
			m.cursor = n - 1
		}
		if m.cursor >= m.offset+visible {
			m.offset = m.cursor - visible + 1
		}
		return m, m.previewCurrentCmd()

	case "ctrl+u":
		m.cursor -= visible / 2
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor < m.offset {
			m.offset = m.cursor
		}
		return m, m.previewCurrentCmd()

	case "G":
		if n > 0 {
			m.cursor = n - 1
			m.offset = n - visible
			if m.offset < 0 {
				m.offset = 0
			}
		}
		return m, m.previewCurrentCmd()

	case "g":
		m.cursor = 0
		m.offset = 0
		return m, m.previewCurrentCmd()
	}

	// Activation key / tab: cycle through popups.
	key := msg.String()
	if key == m.activationKey || key == "tab" {
		return m, cyclePopup("history", m.cyclePattern, m.commandFile, true)
	}
	if key == m.shiftActivationKey || key == "shift+tab" {
		return m, cyclePopup("history", m.cyclePattern, m.commandFile, false)
	}

	return m, nil
}

// visibleLines returns the number of entry rows that fit in the popup.
// Matches the +7 overhead used by calcHistoryHeight (and all other dynamic
// popup height calcs).
func (m HistoryModel) visibleLines() int {
	v := m.height - 5
	if v < 1 {
		v = 1
	}
	return v
}

// renderRow formats one history entry as a fixed-width plain string.
// Using fmt.Sprintf (no ANSI sequences) lets the cursor style be applied
// cleanly to the whole row as a single background span.
func renderRow(sessName, winName, timeStr string) string {
	return fmt.Sprintf("%-*s  %-*s  %-*s%*s",
		histSessColW, sessName,
		histWinColW, winName,
		histTimeColW, timeStr,
		histRightPad, "")
}

func (m HistoryModel) View() string {
	var sb strings.Builder

	// Blank line at top.
	sb.WriteString("\n")
	if errLine := keysErrorLine(m.width); errLine != "" {
		sb.WriteString(errLine + "\n")
	}

	// Column headers.
	sessHead := lipgloss.NewStyle().Width(histSessColW).Render(histHeaderStyle.Render("Session"))
	winHead := lipgloss.NewStyle().Width(histWinColW).Render(histHeaderStyle.Render("Window"))
	timeHead := histHeaderStyle.Render("Visited")
	sb.WriteString(histPad + sessHead + "  " + winHead + "  " + timeHead + "\n")

	// Rule — same width as a rendered row.
	sb.WriteString(histPad + histRuleStyle.Render(strings.Repeat("─", histRuleW)) + "\n")

	// Entry rows.
	if len(m.entries) == 0 {
		sb.WriteString(lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).
			Render(dimStyle.Render("No history yet")) + "\n")
	} else {
		visible := m.visibleLines()
		end := m.offset + visible
		if end > len(m.entries) {
			end = len(m.entries)
		}
		for i := m.offset; i < end; i++ {
			e := m.entries[i]
			row := renderRow(
				truncate(e.SessionName, histSessColW),
				truncate(e.WindowName, histWinColW),
				formatRelativeAge(e.Visited),
			)
			if i == m.cursor {
				sb.WriteString(histPad + histCursorBg.Render(row) + "\n")
			} else {
				sb.WriteString(histPad + row + "\n")
			}
		}
	}

	content := sb.String()

	// Hint bar.
	total := len(m.entries)
	visible := m.visibleLines()
	var hint string
	if total > visible {
		pct := (m.offset + visible) * 100 / total
		if pct > 100 {
			pct = 100
		}
		hint = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(
			hintStyle.Render(fmt.Sprintf("[j/k] move  [enter] switch  [q] close  %d%%", pct)))
	} else {
		hint = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(
			hintStyle.Render("[enter] switch  [q] close"))
	}

	// Dynamic padding pushes the hint to the bottom of the popup, matching
	// the approach used by Model.View() and SlotsModel.View().
	padding := m.height - strings.Count(content, "\n") - 1
	if padding < 1 {
		padding = 1
	}
	return content + strings.Repeat("\n", padding) + hint
}

// ── Entry point ───────────────────────────────────────────────────────────────

func runHistoryBody(args []string) {
	fs := flag.NewFlagSet("show-history-body", flag.ExitOnError)
	commandFile := fs.String("command-file", "", "write deferred commands here")
	fs.Parse(args)

	activationKey := tmuxGetGlobalOption("@hometown_activation_key")
	if activationKey == "" {
		activationKey = "u"
	}

	m, err := newHistoryModel(*commandFile, activationKey, shiftOf(activationKey), getCyclePattern())
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
