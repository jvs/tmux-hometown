package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// ── Popup cycle ───────────────────────────────────────────────────────────────

// defaultCyclePattern is the order used when @hometown_cycle_pattern is unset.
const defaultCyclePattern = "state,grid,sessions,windows,history"

// popupCommand maps a short popup name to its show-* subcommand.
var popupCommand = map[string]string{
	"state":    "show-state",
	"grid":     "show-grid",
	"sessions": "show-sessions",
	"windows":  "show-windows",
	"history":  "show-history",
}

// getCyclePattern returns the active cycle pattern, falling back to the default.
func getCyclePattern() string {
	if p := tmuxGetGlobalOption("@hometown_cycle_pattern"); p != "" {
		return p
	}
	return defaultCyclePattern
}

// parseCyclePattern splits a comma-separated pattern into recognised popup names.
func parseCyclePattern(pattern string) []string {
	var out []string
	for _, part := range strings.Split(pattern, ",") {
		name := strings.TrimSpace(part)
		if _, ok := popupCommand[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

// cycle returns the show-* command adjacent to name in pattern.
// If forward is true it returns the next item; otherwise the previous.
// Returns "" if name is not found in the pattern or the pattern is empty.
func cycle(name, pattern string, forward bool) string {
	items := parseCyclePattern(pattern)
	for i, item := range items {
		if item == name {
			var j int
			if forward {
				j = (i + 1) % len(items)
			} else {
				j = (i - 1 + len(items)) % len(items)
			}
			return popupCommand[items[j]]
		}
	}
	return ""
}

// laneOrder contains the configured lane keys in order, e.g. ["h","j","k","l",";"].
// Populated by initKeys() at startup via buildKeyState.
var laneOrder []string

type Session struct {
	ID   string
	Name string
}

type Window struct {
	ID        string
	Name      string
	Index     string
	Lane      int // lane index (0–4); -1 means unassigned
	SessionID string
}

func getCurrentSessionAndWindow() (sessID, winID string, err error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_id} #{window_id}").Output()
	if err != nil {
		return "", "", fmt.Errorf("display-message: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected output: %q", string(out))
	}
	return parts[0], parts[1], nil
}

func loadSession(sessID string) (Session, error) {
	out, err := exec.Command("tmux", "display-message", "-t", sessID, "-p",
		"#{session_id} #{session_name}").Output()
	if err != nil {
		return Session{}, fmt.Errorf("display-message: %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), " ", 2)
	if len(parts) != 2 {
		return Session{}, fmt.Errorf("unexpected output: %q", string(out))
	}
	return Session{ID: parts[0], Name: parts[1]}, nil
}

func listAllSessions() ([]Session, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_id} #{session_name}").Output()
	if err != nil {
		return nil, fmt.Errorf("list-sessions: %w", err)
	}
	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		sessions = append(sessions, Session{ID: parts[0], Name: parts[1]})
	}
	return sessions, nil
}

func loadWindows(sessID string) ([]Window, error) {
	// #{@hometown_lane} comes before #{window_name} so names with spaces are captured by SplitN.
	out, err := exec.Command("tmux", "list-windows", "-t", sessID, "-F",
		"#{window_id} #{window_index} #{@hometown_lane} #{window_name}").Output()
	if err != nil {
		return nil, err
	}
	var windows []Window
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 4 {
			continue
		}
		laneIdx := parseIndex(parts[2]) // -1 if unset or unrecognised
		windows = append(windows, Window{
			ID:        parts[0],
			Index:     parts[1],
			Lane:      laneIdx,
			Name:      parts[3],
			SessionID: sessID,
		})
	}
	return windows, nil
}

// groupByLane groups windows into per-lane slices preserving tmux window order.
func groupByLane(windows []Window) map[int][]Window {
	lanes := make(map[int][]Window, len(laneOrder))
	for i := range laneOrder {
		lanes[i] = nil
	}
	for _, w := range windows {
		if w.Lane < 0 {
			continue // unassigned
		}
		lanes[w.Lane] = append(lanes[w.Lane], w)
	}
	return lanes
}

func tmuxRun(args ...string) error {
	return exec.Command("tmux", args...).Run()
}

// tmuxRunShell executes a tmux command string that may contain "; "-separated
// sub-commands (the same format used by confirm-before). Each sub-command is
// split on whitespace and chained with tmux's \; separator.
func tmuxRunShell(cmdStr string) error {
	parts := strings.Split(cmdStr, "; ")
	var argv []string
	for i, part := range parts {
		if i > 0 {
			argv = append(argv, ";")
		}
		argv = append(argv, strings.Fields(part)...)
	}
	return exec.Command("tmux", argv...).Run()
}

// ── Global option helpers ─────────────────────────────────────────────────────

func tmuxGetGlobalOption(name string) string {
	out, err := exec.Command("tmux", "show-option", "-gqv", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func tmuxSetGlobalOption(name, value string) error {
	return exec.Command("tmux", "set-option", "-g", name, value).Run()
}

func tmuxUnsetGlobalOption(name string) error {
	return exec.Command("tmux", "set-option", "-gu", name).Run()
}

// ── Session option helpers ────────────────────────────────────────────────────

func tmuxGetSessionOption(sessID, name string) string {
	out, err := exec.Command("tmux", "show-option", "-t", sessID, "-qv", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func tmuxSetSessionOption(sessID, name, value string) error {
	return exec.Command("tmux", "set-option", "-t", sessID, name, value).Run()
}

// ── Window option helpers ─────────────────────────────────────────────────────

// tmuxGetCurrentWindowOption reads an option from the currently active window.
func tmuxGetCurrentWindowOption(name string) string {
	out, err := exec.Command("tmux", "show-option", "-wqv", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ── Lane helpers ──────────────────────────────────────────────────────────────

// getCurrentLane returns the @hometown_lane option of the currently active window.
func getCurrentLane() int {
	i := parseIndex(tmuxGetCurrentWindowOption("@hometown_lane"))
	if i < 0 {
		return 1
	}
	return i
}

// mostRecentWindowInLane returns the window in the given lane with the highest
// @hometown_visited timestamp.  Falls back to the first window in the lane when
// none have a visit record.  Returns nil if the lane has no windows.
func mostRecentWindowInLane(windows []Window, lane int) *Window {
	laneWins := filterByLane(windows, lane)
	if len(laneWins) == 0 {
		return nil
	}
	best := &laneWins[0]
	bestTS := getWindowVisitTS(laneWins[0].ID)
	for i := 1; i < len(laneWins); i++ {
		ts := getWindowVisitTS(laneWins[i].ID)
		if ts > bestTS {
			bestTS = ts
			best = &laneWins[i]
		}
	}
	return best
}

// sessionExists checks whether a session ID exists.
func sessionExists(sessID string) bool {
	return exec.Command("tmux", "has-session", "-t", sessID).Run() == nil
}

// filterByLane returns windows in the given lane.
func filterByLane(windows []Window, lane int) []Window {
	var result []Window
	for _, w := range windows {
		if w.Lane == lane {
			result = append(result, w)
		}
	}
	return result
}

// indexByID returns the index of the window with the given ID, or -1 if not found.
func indexByID(windows []Window, id string) int {
	for i, w := range windows {
		if w.ID == id {
			return i
		}
	}
	return -1
}

// shellSingleQuote wraps s in single quotes, escaping any single quotes within
// s using backslash. The result is safe to embed in a shell script regardless
// of what characters s contains.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
