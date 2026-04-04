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
		fmt.Fprintln(os.Stderr, "Usage: hometown <command> [args]")
		fmt.Fprintln(os.Stderr, "Commands: switch-window, switch-session, flip-window, flip-session,")
		fmt.Fprintln(os.Stderr, "          show-windows, show-sessions, show-all,")
		fmt.Fprintln(os.Stderr, "          new-window, kill-window, kill-session,")
		fmt.Fprintln(os.Stderr, "          previous-session, next-session,")
		fmt.Fprintln(os.Stderr, "          previous-window-in-current-session, next-window-in-current-session,")
		fmt.Fprintln(os.Stderr, "          previous-window-in-any-session, next-window-in-any-session")
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "switch-window":
		if len(args) < 1 {
			die("switch-window requires a key (h, j, k, l, or ;)")
		}
		key, err := parseStoreKey(args[0])
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
		key, err := parseStoreKey(args[0])
		if err != nil {
			die("%v", err)
		}
		if err := cmdSwitchStore(key); err != nil {
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

	case "show-all":
		if err := cmdShowPopup("all"); err != nil {
			die("%v", err)
		}

	case "switch-session-and-show-lanes":
		if len(args) < 1 {
			die("switch-session-and-show-lanes requires a key (h, j, k, l, or ;)")
		}
		key, err := parseStoreKey(args[0])
		if err != nil {
			die("%v", err)
		}
		if err := cmdSwitchStoreAndShowLanes(key); err != nil {
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
		runStoresBody(args)

	case "show-all-body":
		runAllBody(args)

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
	case "windows":
		body = fmt.Sprintf(
			"#!/bin/sh\nexec %s show-windows-body --command-file %s --return-view windows --switch-view sessions\n",
			exe, popupCmdFile)
	case "sessions":
		body = fmt.Sprintf(
			"#!/bin/sh\nexec %s show-sessions-body --command-file %s --return-view sessions\n",
			exe, popupCmdFile)
	case "all":
		body = fmt.Sprintf(
			"#!/bin/sh\nexec %s show-all-body --command-file %s\n",
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
		height := calcStoresHeight()
		return append(base,
			"-h", fmt.Sprintf("%d", height),
			"-w", "90",
			"-T", "#[align=centre fg=white] Sessions ",
			"-EE", scriptPath,
		)

	case "all":
		return append(base,
			"-h", "16",
			"-w", "90",
			"-T", "#[align=centre fg=white] All ",
			"-EE", scriptPath,
		)

	}
	return append(base, "-EE", scriptPath)
}

func calcStoresHeight() int {
	stores := groupByStore()
	maxPerStore := 1
	for _, key := range storeKeys {
		if n := len(stores[key]); n > maxPerStore {
			maxPerStore = n
		}
	}
	h := maxPerStore + 7
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
	currentLaneIsDefault := tmuxGetCurrentWindowOption("@lane") == ""

	// Save current window for the current lane before doing anything.
	tmuxSetSessionOption(sessID, "@lane_"+currentLane+"_window", curWinID)

	// If the current window has no @lane set, assign it now.
	if currentLaneIsDefault {
		tmuxRun("set-window-option", "-t", curWinID, "@lane", currentLane)
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
		tmuxSetSessionOption(sessID, "@lane_"+key+"_window", nextWin.ID)
		if err := tmuxRun("select-window", "-t", nextWin.ID); err != nil {
			return err
		}
		recordWindowVisit(nextWin.ID)
		return nil
	}

	// Switching to a different lane.
	tmuxSetSessionOption(sessID, "@hometown_flip_window", currentLane)

	targetWinID := tmuxGetSessionOption(sessID, "@lane_"+key+"_window")
	if targetWinID != "" && windowExists(targetWinID) {
		if err := tmuxRun("select-window", "-t", targetWinID); err != nil {
			return err
		}
		recordWindowVisit(targetWinID)
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
	tmuxRun("set-window-option", "-t", newWinID, "@lane", key)
	tmuxSetSessionOption(sessID, "@lane_"+key+"_window", newWinID)
	recordWindowVisit(newWinID)
	return nil
}

func cmdFlipWindow() error {
	sessID, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}

	currentLane := getCurrentLane()
	prevLane := tmuxGetSessionOption(sessID, "@hometown_flip_window")

	if prevLane == "" || prevLane == currentLane {
		return tmuxRun("display-message", "No flip window")
	}

	// Save current window and swap prev/current.
	tmuxSetSessionOption(sessID, "@lane_"+currentLane+"_window", curWinID)
	tmuxSetSessionOption(sessID, "@hometown_flip_window", currentLane)

	tmuxRun("display-message", "[ Window "+laneDisplayLabel(prevLane)+" ]")

	targetWinID := tmuxGetSessionOption(sessID, "@lane_"+prevLane+"_window")
	if targetWinID != "" && windowExists(targetWinID) {
		if err := tmuxRun("select-window", "-t", targetWinID); err != nil {
			return err
		}
		recordWindowVisit(targetWinID)
		return nil
	}

	// No window in previous target lane — create one.
	out, err := exec.Command("tmux", "new-window",
		"-c", "#{pane_current_path}",
		"-P", "-F", "#{window_id}").Output()
	if err != nil {
		return err
	}
	newWinID := strings.TrimSpace(string(out))
	tmuxRun("set-window-option", "-t", newWinID, "@lane", prevLane)
	tmuxSetSessionOption(sessID, "@lane_"+prevLane+"_window", newWinID)
	recordWindowVisit(newWinID)
	return nil
}

func cmdNewWindow() error {
	sessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}

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
	tmuxRun("set-window-option", "-t", newWinID, "@lane", currentLane)
	tmuxSetSessionOption(sessID, "@lane_"+currentLane+"_window", newWinID)
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
//  1. Another window in the same lane (also updates @lane_X_window).
//  2. Any other window in the session.
//  3. Another session entirely (when this is the last window).
//  4. No switch needed (only session — tmux will exit).
func buildKillWindowConfirmCmd(sessID, curWinID, currentLane string, windows []Window) string {
	for _, w := range filterByLane(windows, currentLane) {
		if w.ID != curWinID {
			return fmt.Sprintf(
				"kill-window; select-window -t %s; set-option @lane_%s_window %s",
				w.ID, currentLane, w.ID)
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
	fallbackID := findFallbackSessionID(sessID, all)
	// kill-window on the last window also kills the session, so switching
	// the client away first keeps it alive.
	return fmt.Sprintf("switch-client -t %s; kill-window -t %s", fallbackID, curWinID)
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
		fallbackID := findFallbackSessionID(sessID, all)
		confirmCmd = fmt.Sprintf("switch-client -t %s; kill-session -t %s", fallbackID, sessID)
	}
	return tmuxRun("confirm-before", "-p",
		fmt.Sprintf(" Kill session %q?", sess.Name), confirmCmd)
}

func cmdTagNewWindow() error {
	existing := tmuxGetCurrentWindowOption("@lane")
	if existing != "" {
		return nil
	}
	// Inherit lane from the previous window, defaulting to "j".
	out, err := exec.Command("tmux", "show-option", "-wqv", "-t", "{last}", "@lane").Output()
	lane := "j"
	if err == nil {
		if l := strings.TrimSpace(string(out)); l != "" {
			lane = l
		}
	}
	return exec.Command("tmux", "set-option", "-w", "@lane", lane).Run()
}

// ── Store commands ────────────────────────────────────────────────────────────

func cmdSwitchStore(key string) error {
	currentSessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}

	currentKey := storeKeyForSession(currentSessID)
	if currentKey != "" && currentKey != key {
		tmuxSetGlobalOption("@hometown_flip_session", currentKey)
	}

	sessions := getStoreSessions(key)

	if len(sessions) == 0 {
		// No session for this store — create one.
		newSessID, err := newStoreSession(key)
		if err != nil {
			return err
		}
		if err := setStore(key, newSessID); err != nil {
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
			tmuxRun("display-message", "[ Session "+storeDisplayNames[key]+" ]")
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

// cmdSwitchStoreAndShowLanes switches to a store and opens the lanes popup
// explicitly targeting a pane in the new session, so that display-popup and
// its format-string expansion use the correct session context rather than
// the inherited $TMUX_PANE from the original session.
func cmdSwitchStoreAndShowLanes(key string) error {
	currentSessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}

	// Update flip-session tracking.
	currentKey := storeKeyForSession(currentSessID)
	if currentKey != "" && currentKey != key {
		tmuxSetGlobalOption("@hometown_flip_session", currentKey)
	}

	// Get or create the target session (use first in the store).
	sessions := getStoreSessions(key)
	var targetSessID string
	if len(sessions) > 0 {
		targetSessID = sessions[0].ID
	} else {
		targetSessID, err = newStoreSession(key)
		if err != nil {
			return err
		}
		if err := setStore(key, targetSessID); err != nil {
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
	prevKey := tmuxGetGlobalOption("@hometown_flip_session")
	if prevKey == "" {
		return tmuxRun("display-message", "No flip session")
	}
	return cmdSwitchStore(prevKey)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// laneDisplayLabel returns a display-friendly label for a lane key.
func laneDisplayLabel(key string) string {
	if key == "semi" {
		return ";"
	}
	return strings.ToUpper(key)
}
