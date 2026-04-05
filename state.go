package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	stateHeadStyle = lipgloss.NewStyle().Bold(true)
	stateRuleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stateKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
)

// StateModel is a read-only, scrollable view of every hometown option value
// stored in the running tmux server — useful for debugging configuration.
type StateModel struct {
	lines              []string
	offset             int // index of first visible line
	width              int
	height             int
	commandFile        string
	activationKey      string
	shiftActivationKey string
}

func newStateModel(commandFile, activationKey, shiftActivationKey string) StateModel {
	return StateModel{
		lines:              buildStateLines(),
		width:              88,
		height:             22,
		commandFile:        commandFile,
		activationKey:      activationKey,
		shiftActivationKey: shiftActivationKey,
	}
}

func (m StateModel) Init() tea.Cmd { return nil }

func (m StateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m StateModel) handleKey(msg tea.KeyMsg) (StateModel, tea.Cmd) {
	visible := m.visibleLines()
	maxOffset := len(m.lines) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	switch msg.String() {
	case "q", "esc", "enter":
		return m, tea.Quit
	case "j", "down":
		if m.offset < maxOffset {
			m.offset++
		}
	case "k", "up":
		if m.offset > 0 {
			m.offset--
		}
	case "ctrl+d":
		m.offset += visible / 2
		if m.offset > maxOffset {
			m.offset = maxOffset
		}
	case "ctrl+u":
		m.offset -= visible / 2
		if m.offset < 0 {
			m.offset = 0
		}
	case "G":
		m.offset = maxOffset
	case "g":
		m.offset = 0
	}
	// Activation key: plain → show-history; shift → show-windows.
	if msg.String() == m.activationKey {
		if m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-history\n"), 0644)
		}
		return m, tea.Quit
	}
	if msg.String() == m.shiftActivationKey {
		if m.commandFile != "" {
			exe, _ := os.Executable()
			os.WriteFile(m.commandFile, []byte(exe+" show-windows\n"), 0644)
		}
		return m, tea.Quit
	}
	return m, nil
}

// visibleLines returns the number of content lines that fit in the popup,
// reserving one line at the top (blank) and one at the bottom (hint bar).
func (m StateModel) visibleLines() int {
	v := m.height - 2
	if v < 1 {
		v = 1
	}
	return v
}

func (m StateModel) View() string {
	visible := m.visibleLines()
	end := m.offset + visible
	if end > len(m.lines) {
		end = len(m.lines)
	}

	shown := make([]string, visible)
	copy(shown, m.lines[m.offset:end])

	content := "\n" + strings.Join(shown, "\n")

	// Scroll percentage and hint bar.
	total := len(m.lines)
	var hint string
	if total > visible {
		pct := (m.offset + visible) * 100 / total
		if pct > 100 {
			pct = 100
		}
		hint = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(
			hintStyle.Render(fmt.Sprintf("[j/k] scroll · [ctrl+d/u] half-page · [q] close · %d%%", pct)))
	} else {
		hint = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(
			hintStyle.Render("[q] close"))
	}

	return content + "\n" + hint
}

// ── Data gathering ────────────────────────────────────────────────────────────

func buildStateLines() []string {
	const ruleWidth = 82
	rule := "   " + stateRuleStyle.Render(strings.Repeat("─", ruleWidth))

	var lines []string

	// ── Build ───────────────────────────────────────────────────────────────
	lines = append(lines, "   "+stateHeadStyle.Render("Build"))
	lines = append(lines, rule)
	exe, exeErr := os.Executable()
	lines = append(lines,
		fmt.Sprintf("   %s  %s",
			stateKeyStyle.Render(fmt.Sprintf("%-22s", "version")),
			version))
	if exeErr == nil {
		lines = append(lines,
			fmt.Sprintf("   %s  %s",
				stateKeyStyle.Render(fmt.Sprintf("%-22s", "path")),
				leftTruncate(exe, 58)))
		if info, statErr := os.Stat(exe); statErr == nil {
			lines = append(lines,
				fmt.Sprintf("   %s  %s",
					stateKeyStyle.Render(fmt.Sprintf("%-22s", "modified")),
					formatModTime(info.ModTime())))
		}
	}
	lines = append(lines, "")

	// ── Configuration ────────────────────────────────────────────────────────
	lines = append(lines, "   "+stateHeadStyle.Render("Configuration"))
	lines = append(lines, rule)
	rawActivationKey := tmuxGetGlobalOption("@hometown_activation_key")
	activationKeyVal := rawActivationKey
	if activationKeyVal == "" {
		activationKeyVal = "u"
	}
	activationKeyDisplay := activationKeyVal
	if rawActivationKey == "" {
		activationKeyDisplay += "  " + dimStyle.Render("(default)")
	}
	lines = append(lines,
		fmt.Sprintf("   %s  %s",
			stateKeyStyle.Render(fmt.Sprintf("%-22s", "activation_key")),
			activationKeyDisplay))
	lines = append(lines, "")

	// ── Sessions ──────────────────────────────────────────────────────────────
	lines = append(lines, "   "+stateHeadStyle.Render("Sessions"))
	lines = append(lines, rule)
	// Tab-delimited; session name is last so it can contain spaces.
	out, err := exec.Command("tmux", "list-sessions", "-F",
		"#{session_id}\t#{@hometown_slot}\t#{@hometown_slot_never}\t#{session_name}",
	).Output()
	if err == nil {
		for _, p := range splitTabLines(string(out), 4) {
			sessID, slotKey, slotNever, name := p[0], p[1], p[2], p[3]
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("   %-6s  %-22s", sessID, truncate(name, 22)))
			appendStateOpt(&sb, "@hometown_slot", slotKey)
			appendStateOpt(&sb, "@hometown_slot_never", slotNever)
			lines = append(lines, sb.String())
		}
	}
	lines = append(lines, "")

	// ── Windows ───────────────────────────────────────────────────────────────
	lines = append(lines, "   "+stateHeadStyle.Render("Windows"))
	lines = append(lines, rule)

	// Build session ID → name and slot maps.
	sessNames := map[string]string{}
	sessSlots := map[string]string{}
	if nameOut, nameErr := exec.Command("tmux", "list-sessions", "-F",
		"#{session_id}\t#{@hometown_slot}\t#{session_name}",
	).Output(); nameErr == nil {
		for _, p := range splitTabLines(string(nameOut), 3) {
			sessNames[p[0]] = p[2]
			sessSlots[p[0]] = p[1]
		}
	}

	// Window name is last.
	out, err = exec.Command("tmux", "list-windows", "-a", "-F",
		"#{session_id}\t#{window_id}\t#{@hometown_lane}\t#{@hometown_visited}\t#{@hometown_lane_never}\t#{window_name}",
	).Output()
	if err == nil {
		const sessNameColW = 22
		prevSess := ""
		for _, p := range splitTabLines(string(out), 6) {
			sessID, winID, lane, visited, laneNever, name := p[0], p[1], p[2], p[3], p[4], p[5]
			if sessID != prevSess {
				if prevSess != "" {
					lines = append(lines, "")
				}
				prevSess = sessID
				sessName := sessNames[sessID]
				var sessLabel string
				if slot := sessSlots[sessID]; slot != "" {
					slotStr := "  Slot " + slotDisplayNames[slot]
					if len([]rune(sessName)) <= sessNameColW {
						sessLabel = fmt.Sprintf("%-6s  %-*s%s", sessID, sessNameColW, sessName, slotStr)
					} else {
						sessLabel = fmt.Sprintf("%-6s  %s%s", sessID, sessName, slotStr)
					}
				} else {
					sessLabel = fmt.Sprintf("%-6s  %s", sessID, sessName)
				}
				lines = append(lines, "   "+dimStyle.Render(sessLabel))
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("   %-6s  %-22s", winID, truncate(name, 22)))
			appendStateOpt(&sb, "@hometown_lane", lane)
			if visited != "" && visited != "0" {
				appendStateOptVal(&sb, "@hometown_visited", formatVisitedTS(visited))
			}
			appendStateOpt(&sb, "@hometown_lane_never", laneNever)
			lines = append(lines, sb.String())
		}
	}
	lines = append(lines, "")

	return lines
}

// stripHometown removes the "@hometown_" prefix from an option name for
// compact display in the state view.
func stripHometown(opt string) string {
	return strings.TrimPrefix(strings.TrimPrefix(opt, "@hometown_"), "@")
}
func appendStateOpt(sb *strings.Builder, key, val string) {
	if val == "" {
		return
	}
	appendStateOptVal(sb, key, val)
}

// appendStateOptVal appends " key=val" to sb unconditionally.
func appendStateOptVal(sb *strings.Builder, key, val string) {
	sb.WriteString("  ")
	sb.WriteString(stateKeyStyle.Render(stripHometown(key) + "="))
	sb.WriteString(val)
}

// leftTruncate shortens s to at most max runes, prefixing with '…' if trimmed.
func leftTruncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return "…" + string(runes[len(runes)-(max-1):])
}

// formatModTime formats a file modification time as a human-readable age.
func formatModTime(t time.Time) string {
	ago := time.Since(t)
	switch {
	case ago < time.Minute:
		return "just now"
	case ago < time.Hour:
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	case ago < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(ago.Hours()))
	case ago < 7*24*time.Hour:
		days := int(ago.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("2006-01-02")
	}
}

// formatVisitedTS converts a nanosecond Unix timestamp string to a human-readable age.
func formatVisitedTS(ts string) string {
	var n int64
	if _, err := fmt.Sscanf(ts, "%d", &n); err != nil || n == 0 {
		return ts
	}
	ago := time.Since(time.Unix(0, n))
	switch {
	case ago < time.Minute:
		return fmt.Sprintf("%ds ago", int(ago.Seconds()))
	case ago < time.Hour:
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	default:
		return time.Unix(0, n).Format("15:04:05")
	}
}

// splitTabLines splits s into lines and then each line into exactly n
// tab-separated fields. Lines that don't produce exactly n fields are dropped.
func splitTabLines(s string, n int) [][]string {
	var result [][]string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", n)
		if len(parts) == n {
			result = append(result, parts)
		}
	}
	return result
}

// ── Entry point ───────────────────────────────────────────────────────────────

func runStateBody(args []string) {
	fs := flag.NewFlagSet("show-state-body", flag.ExitOnError)
	commandFile := fs.String("command-file", "", "write deferred commands here")
	fs.Parse(args)

	activationKey := tmuxGetGlobalOption("@hometown_activation_key")
	if activationKey == "" {
		activationKey = "u"
	}

	m := newStateModel(*commandFile, activationKey, shiftOf(activationKey))
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "hometown: %v\n", err)
		os.Exit(1)
	}
}
