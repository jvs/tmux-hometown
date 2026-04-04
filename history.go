package main

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// recordWindowVisit stamps the given window with the current nanosecond time,
// establishing its position in the visit history.
func recordWindowVisit(winID string) {
	ts := strconv.FormatInt(time.Now().UnixNano(), 10)
	exec.Command("tmux", "set-window-option", "-t", winID, "@hometown_visited", ts).Run()
}

// recordActiveWindowVisit records a visit to whichever window is currently
// active in the given session. Used after switch-client when the exact
// window ID is not already known.
func recordActiveWindowVisit(sessID string) {
	out, err := exec.Command("tmux", "display-message", "-t", sessID, "-p", "#{window_id}").Output()
	if err != nil {
		return
	}
	if winID := strings.TrimSpace(string(out)); winID != "" {
		recordWindowVisit(winID)
	}
}

// getWindowVisitTS returns the @hometown_visited timestamp for a specific
// window, or 0 if the window has never been visited via tmux-hometown.
func getWindowVisitTS(winID string) int64 {
	out, err := exec.Command("tmux", "show-option", "-wqv", "-t", winID, "@hometown_visited").Output()
	if err != nil {
		return 0
	}
	ts, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return ts
}

// visitedWindow holds a window's identity alongside its last-visit timestamp.
type visitedWindow struct {
	SessionID string
	WindowID  string
	Visited   int64
}

// listAllWindowVisits returns all windows across all sessions that carry a
// non-zero @hometown_visited stamp.
func listAllWindowVisits() ([]visitedWindow, error) {
	out, err := exec.Command("tmux", "list-windows", "-a", "-F",
		"#{session_id} #{window_id} #{@hometown_visited}").Output()
	if err != nil {
		return nil, err
	}
	var result []visitedWindow
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		// Session IDs ($N), window IDs (@N), and timestamps are all
		// space-free, so Fields is safe here.
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue // unvisited: timestamp field is absent or empty
		}
		ts, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || ts == 0 {
			continue
		}
		result = append(result, visitedWindow{
			SessionID: parts[0],
			WindowID:  parts[1],
			Visited:   ts,
		})
	}
	return result, nil
}

// switchToWindow switches the client to the given window, changing sessions
// first if the window belongs to a different session.
func switchToWindow(w *visitedWindow) error {
	sessID, _, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	if w.SessionID != sessID {
		// switch-client accepts session:window as a single target.
		return tmuxRun("switch-client", "-t", w.SessionID+":"+w.WindowID)
	}
	return tmuxRun("select-window", "-t", w.WindowID)
}

// ── History navigation ────────────────────────────────────────────────────────
//
// All six commands share the same pattern:
//   1. Get the current window's timestamp (its position in history).
//   2. Load all visited windows across all sessions.
//   3. Filter by scope (same/different/any session) and direction (< or >).
//   4. Pick the closest match (max for previous, min for next).
//   5. Switch to it, or show a status message if nothing qualifies.
//
// Navigation commands are read-only: they do NOT call recordWindowVisit,
// so browsing history does not disturb the ordering.

func cmdPreviousWindowInCurrentSession() error {
	sessID, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	curTS := getWindowVisitTS(curWinID)
	if curTS == 0 {
		return tmuxRun("display-message", "No visit history")
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
		if w.Visited < curTS && (best == nil || w.Visited > best.Visited) {
			best = w
		}
	}
	if best == nil {
		return tmuxRun("display-message", "No previous window")
	}
	return tmuxRun("select-window", "-t", best.WindowID)
}

func cmdNextWindowInCurrentSession() error {
	sessID, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	curTS := getWindowVisitTS(curWinID)
	if curTS == 0 {
		return tmuxRun("display-message", "No visit history")
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
		if w.Visited > curTS && (best == nil || w.Visited < best.Visited) {
			best = w
		}
	}
	if best == nil {
		return tmuxRun("display-message", "No next window")
	}
	return tmuxRun("select-window", "-t", best.WindowID)
}

func cmdPreviousWindowInAnySession() error {
	_, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	curTS := getWindowVisitTS(curWinID)
	if curTS == 0 {
		return tmuxRun("display-message", "No visit history")
	}
	windows, err := listAllWindowVisits()
	if err != nil {
		return err
	}
	var best *visitedWindow
	for i := range windows {
		w := &windows[i]
		if w.WindowID == curWinID {
			continue
		}
		if w.Visited < curTS && (best == nil || w.Visited > best.Visited) {
			best = w
		}
	}
	if best == nil {
		return tmuxRun("display-message", "No previous window")
	}
	return switchToWindow(best)
}

func cmdNextWindowInAnySession() error {
	_, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	curTS := getWindowVisitTS(curWinID)
	if curTS == 0 {
		return tmuxRun("display-message", "No visit history")
	}
	windows, err := listAllWindowVisits()
	if err != nil {
		return err
	}
	var best *visitedWindow
	for i := range windows {
		w := &windows[i]
		if w.WindowID == curWinID {
			continue
		}
		if w.Visited > curTS && (best == nil || w.Visited < best.Visited) {
			best = w
		}
	}
	if best == nil {
		return tmuxRun("display-message", "No next window")
	}
	return switchToWindow(best)
}

func cmdPreviousSession() error {
	sessID, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	curTS := getWindowVisitTS(curWinID)
	if curTS == 0 {
		return tmuxRun("display-message", "No visit history")
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
		if w.Visited < curTS && (best == nil || w.Visited > best.Visited) {
			best = w
		}
	}
	if best == nil {
		return tmuxRun("display-message", "No previous session")
	}
	return switchToWindow(best)
}

func cmdNextSession() error {
	sessID, curWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		return err
	}
	curTS := getWindowVisitTS(curWinID)
	if curTS == 0 {
		return tmuxRun("display-message", "No visit history")
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
		if w.Visited > curTS && (best == nil || w.Visited < best.Visited) {
			best = w
		}
	}
	if best == nil {
		return tmuxRun("display-message", "No next session")
	}
	return switchToWindow(best)
}
