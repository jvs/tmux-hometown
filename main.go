package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var version = "dev"

const popupCmdFile = "/tmp/hometown_command"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println("tmux-hometown", version)
		return
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "switch-window":
		if len(args) < 1 {
			die("switch-window requires a key (h, j, k, l, or ;)")
		}
		key, err := parseSlotKey(args[0])
		if err != nil {
			die("%v", err)
		}
		if err := cmdSwitchLane(key); err != nil {
			die("%v", err)
		}

	case "switch-session":
		if len(args) < 1 {
			die("switch-session requires a key (h, j, k, l, or ;)")
		}
		key, err := parseSlotKey(args[0])
		if err != nil {
			die("%v", err)
		}
		if err := cmdSwitchSlot(key); err != nil {
			die("%v", err)
		}

	case "flip-window":
		if err := cmdFlipWindow(); err != nil {
			die("%v", err)
		}

	case "flip-session":
		if err := cmdFlipSession(); err != nil {
			die("%v", err)
		}

	case "new-window":
		if err := cmdNewWindow(); err != nil {
			die("%v", err)
		}

	case "kill-window":
		if err := cmdKillWindow(); err != nil {
			die("%v", err)
		}

	case "kill-session":
		if err := cmdKillSession(); err != nil {
			die("%v", err)
		}

	case "show-windows":
		if err := cmdShowPopup("windows"); err != nil {
			die("%v", err)
		}

	case "show-grid":
		if err := cmdShowPopup("grid"); err != nil {
			die("%v", err)
		}

	case "switch-session-and-show-lanes":
		if len(args) < 1 {
			die("switch-session-and-show-lanes requires a key (h, j, k, l, or ;)")
		}
		key, err := parseSlotKey(args[0])
		if err != nil {
			die("%v", err)
		}
		if err := cmdSwitchSlotAndShowLanes(key); err != nil {
			die("%v", err)
		}

	case "show-sessions":
		if err := cmdShowPopup("sessions"); err != nil {
			die("%v", err)
		}

	// Internal: body commands run inside the tmux popup.
	case "show-windows-body":
		runWindowsBody(args)

	case "show-sessions-body":
		runSlotsBody(args)

	case "show-grid-body":
		runGridBody(args)

	case "tag-new-window":
		if err := cmdTagNewWindow(); err != nil {
			die("%v", err)
		}

	case "previous-session":
		if err := cmdPreviousSession(); err != nil {
			die("%v", err)
		}

	case "next-session":
		if err := cmdNextSession(); err != nil {
			die("%v", err)
		}

	case "previous-window-in-current-session":
		if err := cmdPreviousWindowInCurrentSession(); err != nil {
			die("%v", err)
		}

	case "next-window-in-current-session":
		if err := cmdNextWindowInCurrentSession(); err != nil {
			die("%v", err)
		}

	case "previous-window-in-any-session":
		if err := cmdPreviousWindowInAnySession(); err != nil {
			die("%v", err)
		}

	case "next-window-in-any-session":
		if err := cmdNextWindowInAnySession(); err != nil {
			die("%v", err)
		}

	case "show-history":
		if err := cmdShowPopup("history"); err != nil {
			die("%v", err)
		}

	case "show-history-body":
		runHistoryBody(args)

	case "show-state":
		if err := cmdShowPopup("state"); err != nil {
			die("%v", err)
		}

	case "show-state-body":
		runStateBody(args)

	case "record-visit":
		if len(args) >= 1 {
			recordWindowVisit(args[0])
		} else {
			_, winID, err := getCurrentSessionAndWindow()
			if err != nil {
				die("%v", err)
			}
			recordWindowVisit(winID)
		}

	// Internal: stamp a window with the current visit time. Used by deferred
	// shell scripts (commandFile) that create windows outside the Go process.
	case "record-window-visit":
		if len(args) < 1 {
			die("record-window-visit requires a window ID")
		}
		recordWindowVisit(args[0])

	default:
		die("unknown command: %s", cmd)
	}
}

func printUsage() {
	type entry struct {
		name string
		desc string
	}
	type group struct {
		title   string
		entries []entry
	}

	// Keep the Commands section in README.md in sync with this list.
	groups := []group{
		{
			"Navigation",
			[]entry{
				{"switch-window <h|j|k|l|;>", "Switch to that lane; cycle within it, or create a new window"},
				{"switch-session <h|j|k|l|;>", "Switch to that slot; cycle if multiple, or create a new session"},
				{"flip-window", "Toggle back to the previously active window in this session"},
				{"flip-session", "Toggle back to the previously active session"},
				{"new-window", "Create a new window in the current lane"},
				{"kill-window", "Kill the current window (with confirmation)"},
				{"kill-session", "Kill the current session (with confirmation)"},
			},
		},
		{
			"History",
			[]entry{
				{"previous-window-in-current-session", "Go to the most recently visited other window in this session"},
				{"next-window-in-current-session", "Go forward in this session's window history"},
				{"previous-window-in-any-session", "Go to the most recently visited other window across all sessions"},
				{"next-window-in-any-session", "Go forward in the global window history"},
				{"previous-session", "Go to the most recently visited window in a different session"},
				{"next-session", "Go forward to the next session in history"},
			},
		},
		{
			"Popups",
			[]entry{
				{"show-windows", "Open (or close) the windows popup"},
				{"show-sessions", "Open (or close) the sessions popup"},
				{"show-grid", "Open (or close) the grid popup"},
				{"show-history", "Open (or close) the history popup"},
				{"show-state", "Open (or close) the state popup (debug view)"},
			},
		},
		{
			"Utility",
			[]entry{
				{"record-visit [window-id]", "Tell hometown a window was visited (call from other tools, e.g. fzf switchers, so history commands stay accurate)"},
				{"version, --version", "Show the current version"},
			},
		},
	}

	const (
		bold  = "\033[1m"
		cyan  = "\033[36m"
		dim   = "\033[2m"
		reset = "\033[0m"
	)

	fmt.Fprintf(os.Stderr, "\n%sUsage:%s hometown <command> [args]\n", bold, reset)

	for _, g := range groups {
		fmt.Fprintf(os.Stderr, "\n%s%s%s\n", bold+cyan, g.title, reset)

		maxWidth := 0
		for _, e := range g.entries {
			if len(e.name) > maxWidth {
				maxWidth = len(e.name)
			}
		}

		for _, e := range g.entries {
			pad := strings.Repeat(" ", maxWidth-len(e.name))
			fmt.Fprintf(os.Stderr, "  %s%s%s%s  %s%s%s\n",
				bold, e.name, reset, pad,
				dim, e.desc, reset)
		}
	}
	fmt.Fprintln(os.Stderr)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "hometown: "+format+"\n", args...)
	os.Exit(1)
}

// ── Popup lifecycle ───────────────────────────────────────────────────────────

func cmdShowPopup(view string) error {
	currentView := tmuxGetGlobalOption("@hometown_popup_view")

	if currentView == view {
		// Same view is open: clear the option first, then close the popup.
		tmuxSetGlobalOption("@hometown_popup_view", "")
		return tmuxRun("display-popup", "-C")
	}

	if currentView != "" {
		// A different view is open: clear and close it, then open the new one.
		tmuxSetGlobalOption("@hometown_popup_view", "")
		tmuxRun("display-popup", "-C")
	}

	exe, err := os.Executable()
	if err != nil {
		exe = "hometown"
	}

	// Write body script (avoids shell quoting issues with multi-word flags).
	scriptPath := "/tmp/hometown_" + view + "_popup.sh"
	if err := writePopupScript(scriptPath, view, exe); err != nil {
		return fmt.Errorf("writing popup script: %w", err)
	}

	os.Remove(popupCmdFile)
	tmuxSetGlobalOption("@hometown_popup_view", view)

	tmuxRun(buildPopupArgs(view, scriptPath, exe)...)

	// Clear the view option BEFORE running the pending command, so that any
	// show-* command in the pending script sees a clean slate.
	tmuxSetGlobalOption("@hometown_popup_view", "")

	runPendingCommand()
	return nil
}

func writePopupScript(path, view, exe string) error {
	var body string
	switch view {
	case "history":
		body = fmt.Sprintf("#!/bin/sh\nexec %s show-history-body --command-file %s\n",
			exe, popupCmdFile)
	case "state":
		body = fmt.Sprintf("#!/bin/sh\nexec %s show-state-body --command-file %s\n",
			exe, popupCmdFile)
	case "windows":
		body = fmt.Sprintf(
			"#!/bin/sh\nexec %s show-windows-body --command-file %s --return-view windows --switch-view sessions\n",
			exe, popupCmdFile)
	case "sessions":
		body = fmt.Sprintf(
			"#!/bin/sh\nexec %s show-sessions-body --command-file %s --return-view sessions\n",
			exe, popupCmdFile)
	case "grid":
		body = fmt.Sprintf(
			"#!/bin/sh\nexec %s show-grid-body --command-file %s\n",
			exe, popupCmdFile)
	default:
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return err
	}
	return os.Chmod(path, 0755)
}

func buildPopupArgs(view, scriptPath, exe string) []string {
	base := []string{"display-popup", "-b", "rounded"}

	switch view {
	case "history":
		height := calcHistoryHeight()
		return append(base,
			"-h", fmt.Sprintf("%d", height),
			"-w", "73",
			"-T", "#[align=centre fg=white] Hometown History ",
			"-EE", scriptPath,
		)
	case "state":
		return append(base,
			"-h", "28",
			"-w", "90",
			"-T", "#[align=centre fg=white] Hometown State ",
			"-EE", scriptPath,
		)
	case "windows":
		height := 12
		if sessID, _, err := getCurrentSessionAndWindow(); err == nil {
			height = calcLanesHeight(sessID)
		}
		return append(base,
			"-h", fmt.Sprintf("%d", height),
			"-w", "90",
			"-T", "#[align=centre fg=white] #{session_name} ",
			"-EE", scriptPath,
		)

	case "sessions":
		height := calcSlotsHeight()
		return append(base,
			"-h", fmt.Sprintf("%d", height),
			"-w", "90",
			"-T", "#[align=centre fg=white] Hometown Sessions ",
			"-EE", scriptPath,
		)

	case "grid":
		return append(base,
			"-h", "12",
			"-w", "91",
			"-T", "#[align=centre fg=white] Hometown Grid ",
			"-EE", scriptPath,
		)

	}
	return append(base, "-EE", scriptPath)
}

func calcSlotsHeight() int {
	slots := groupBySlot()
	maxPerSlot := 1
	for _, key := range slotKeys {
		if n := len(slots[key]); n > maxPerSlot {
			maxPerSlot = n
		}
	}
	h := maxPerSlot + 7
	if h < 8 {
		h = 8
	}
	return h
}

func calcLanesHeight(sessID string) int {
	windows, err := loadWindows(sessID)
	if err != nil {
		return 10
	}
	lanes := groupByLane(windows)
	maxPerLane := 1
	for _, key := range laneOrder {
		if n := len(lanes[key]); n > maxPerLane {
			maxPerLane = n
		}
	}
	h := maxPerLane + 7
	for _, w := range windows {
		if w.Lane == "" {
			// The "N windows not shown" notice adds a blank line + a text
			// line above the normal actions bar.
			h += 2
			break
		}
	}
	if h < 8 {
		h = 8
	}
	return h
}

func runPendingCommand() {
	data, err := os.ReadFile(popupCmdFile)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return
	}
	// Remove file BEFORE executing — the command may itself write a new entry.
	os.Remove(popupCmdFile)
	exec.Command("sh", "-c", string(data)).Run()
}

// ── Lane commands ─────────────────────────────────────────────────────────────

func cmdSwitchLane(key string) error {
	sessID, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}

	currentLane := getCurrentLane()
	currentLaneIsDefault := tmuxGetCurrentWindowOption("@hometown_lane") == ""

	// If the current window has no @hometown_lane set, assign it now.
	if currentLaneIsDefault {
		tmuxRun("set-window-option", "-t", curWinID, "@hometown_lane", currentLane)
	}

	if key == currentLane {
		// Already in this lane — cycle to next window.
		windows, _ := loadWindows(sessID)
		laneWins := filterByLane(windows, key)
		if len(laneWins) <= 1 {
			return tmuxRun("display-message", "[ Window "+laneDisplayLabel(key)+" ]")
		}
		idx := indexByID(laneWins, curWinID)
		nextIdx := (idx + 1) % len(laneWins)
		nextWin := laneWins[nextIdx]
		if err := tmuxRun("select-window", "-t", nextWin.ID); err != nil {
			return err
		}
		recordWindowVisit(nextWin.ID)
		return nil
	}

	// Switching to a different lane.
	windows, _ := loadWindows(sessID)
	if target := mostRecentWindowInLane(windows, key); target != nil {
		if err := tmuxRun("select-window", "-t", target.ID); err != nil {
			return err
		}
		recordWindowVisit(target.ID)
		return nil
	}

	// No window in this lane yet — create one.
	out, err := exec.Command("tmux", "new-window",
		"-c", "#{pane_current_path}",
		"-P", "-F", "#{window_id}").Output()
	if err != nil {
		return err
	}
	newWinID := strings.TrimSpace(string(out))
	tmuxRun("set-window-option", "-t", newWinID, "@hometown_lane", key)
	recordWindowVisit(newWinID)
	return nil
}

func cmdFlipWindow() error {
	sessID, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	windows, err := listAllWindowVisits()
	if err != nil {
		return err
	}
	var best *visitedWindow
	for i := range windows {
		w := &windows[i]
		if w.SessionID != sessID || w.WindowID == curWinID {
			continue
		}
		if best == nil || w.Visited > best.Visited {
			best = &windows[i]
		}
	}
	if best == nil {
		return tmuxRun("display-message", "No flip window")
	}
	if err := tmuxRun("select-window", "-t", best.WindowID); err != nil {
		return err
	}
	return nil
}

func cmdNewWindow() error {
	// Read current lane BEFORE creating the new window (after creation the
	// active window changes).
	currentLane := getCurrentLane()

	out, err := exec.Command("tmux", "new-window",
		"-c", "#{pane_current_path}",
		"-P", "-F", "#{window_id}").Output()
	if err != nil {
		return err
	}
	newWinID := strings.TrimSpace(string(out))
	tmuxRun("set-window-option", "-t", newWinID, "@hometown_lane", currentLane)
	recordWindowVisit(newWinID)
	return nil
}

func cmdKillWindow() error {
	sessID, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	currentLane := getCurrentLane()
	windows, _ := loadWindows(sessID)
	confirmCmd := buildKillWindowConfirmCmd(sessID, curWinID, currentLane, windows)
	return tmuxRun("confirm-before", "-p", " Kill window?", confirmCmd)
}

// buildKillWindowConfirmCmd returns the tmux command string to run when the
// user confirms a kill-window. It picks the safest switch target:
//  1. Another window in the same lane.
//  2. Any other window in the session.
//  3. Another session entirely (when this is the last window).
//  4. No switch needed (only session — tmux will exit).
func buildKillWindowConfirmCmd(sessID, curWinID, currentLane string, windows []Window) string {
	for _, w := range filterByLane(windows, currentLane) {
		if w.ID != curWinID {
			return fmt.Sprintf(
				"kill-window; select-window -t %s",
				w.ID)
		}
	}
	for _, w := range windows {
		if w.ID != curWinID {
			return fmt.Sprintf("kill-window; select-window -t %s", w.ID)
		}
	}
	// Last window in the session — must move to another session first.
	all, _ := listAllSessions()
	if len(all) <= 1 {
		return "kill-window" // only session; tmux will exit
	}
	fallbackTarget := findFallbackTarget(sessID, all)
	// kill-window on the last window also kills the session, so switching
	// the client away first keeps it alive.
	return fmt.Sprintf("switch-client -t %s; kill-window -t %s", fallbackTarget, curWinID)
}

func cmdKillSession() error {
	sessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	sess, _ := loadSession(sessID)
	all, _ := listAllSessions()
	var confirmCmd string
	if len(all) <= 1 {
		confirmCmd = "kill-session"
	} else {
		fallbackTarget := findFallbackTarget(sessID, all)
		confirmCmd = fmt.Sprintf("switch-client -t %s; kill-session -t %s", fallbackTarget, sessID)
	}
	return tmuxRun("confirm-before", "-p",
		fmt.Sprintf(" Kill session %q?", sess.Name), confirmCmd)
}

func cmdTagNewWindow() error {
	existing := tmuxGetCurrentWindowOption("@hometown_lane")
	if existing != "" {
		return nil
	}
	// Inherit lane from the previous window, defaulting to "j".
	out, err := exec.Command("tmux", "show-option", "-wqv", "-t", "{last}", "@hometown_lane").Output()
	lane := "j"
	if err == nil {
		if l := strings.TrimSpace(string(out)); l != "" {
			lane = l
		}
	}
	return exec.Command("tmux", "set-option", "-w", "@hometown_lane", lane).Run()
}

// ── Slot commands ─────────────────────────────────────────────────────────────

func cmdSwitchSlot(key string) error {
	currentSessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}

	sessions := getSlotSessions(key)

	if len(sessions) == 0 {
		// No session for this slot — create one.
		newSessID, err := newSlotSession(key)
		if err != nil {
			return err
		}
		if err := setSlot(key, newSessID); err != nil {
			return err
		}
		if err := tmuxRun("switch-client", "-t", newSessID); err != nil {
			return err
		}
		recordActiveWindowVisit(newSessID)
		return nil
	}

	if len(sessions) == 1 {
		if sessions[0].ID == currentSessID {
			tmuxRun("display-message", "[ Session "+slotDisplayNames[key]+" ]")
			return nil
		}
		if err := tmuxRun("switch-client", "-t", sessions[0].ID); err != nil {
			return err
		}
		recordActiveWindowVisit(sessions[0].ID)
		return nil
	}

	// Multiple sessions: find current position and cycle to the next.
	idx := -1
	for i, s := range sessions {
		if s.ID == currentSessID {
			idx = i
			break
		}
	}
	nextIdx := (idx + 1) % len(sessions)
	if err := tmuxRun("switch-client", "-t", sessions[nextIdx].ID); err != nil {
		return err
	}
	recordActiveWindowVisit(sessions[nextIdx].ID)
	return nil
}

// cmdSwitchSlotAndShowLanes switches to a slot and opens the lanes popup
// explicitly targeting a pane in the new session, so that display-popup and
// its format-string expansion use the correct session context rather than
// the inherited $TMUX_PANE from the original session.
func cmdSwitchSlotAndShowLanes(key string) error {
	if _, _, err := getCurrentSessionAndWindow(); err != nil {
		return err
	}

	// Get or create the target session (use first in the slot).
	sessions := getSlotSessions(key)
	var targetSessID string
	if len(sessions) > 0 {
		targetSessID = sessions[0].ID
	} else {
		var err error
		targetSessID, err = newSlotSession(key)
		if err != nil {
			return err
		}
		if err := setSlot(key, targetSessID); err != nil {
			return err
		}
	}

	if err := tmuxRun("switch-client", "-t", targetSessID); err != nil {
		return err
	}

	// Get a pane in the target session using an explicit -t so we are not
	// affected by the inherited $TMUX_PANE pointing to the old session.
	out, err := exec.Command("tmux", "display-message", "-t", targetSessID, "-p", "#{pane_id}").Output()
	if err != nil {
		return err
	}
	paneID := strings.TrimSpace(string(out))

	exe, err := os.Executable()
	if err != nil {
		exe = "hometown"
	}

	scriptPath := "/tmp/hometown_windows_popup.sh"
	if err := writePopupScript(scriptPath, "windows", exe); err != nil {
		return err
	}

	height := calcLanesHeight(targetSessID)

	tmuxSetGlobalOption("@hometown_popup_view", "windows")
	tmuxRun("display-popup",
		"-t", paneID,
		"-b", "rounded",
		"-h", fmt.Sprintf("%d", height),
		"-w", "90",
		"-T", "#[align=centre fg=white] #{session_name} ",
		"-EE", scriptPath,
	)
	tmuxSetGlobalOption("@hometown_popup_view", "")
	runPendingCommand()
	return nil
}

func cmdFlipSession() error {
	sessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	windows, err := listAllWindowVisits()
	if err != nil {
		return err
	}
	var best *visitedWindow
	for i := range windows {
		w := &windows[i]
		if w.SessionID == sessID {
			continue
		}
		if best == nil || w.Visited > best.Visited {
			best = &windows[i]
		}
	}
	if best == nil {
		return tmuxRun("display-message", "No flip session")
	}
	if err := switchToWindow(best); err != nil {
		return err
	}
	return nil
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// laneDisplayLabel returns a display-friendly label for a lane key.
func laneDisplayLabel(key string) string {
	if key == "semi" {
		return ";"
	}
	return strings.ToUpper(key)
}
