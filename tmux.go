package main

import (
	"fmt"
	"os/exec"
	"strings"
)

var laneOrder = []string{"h", "j", "k", "l", "semi"}

var laneLabels = map[string]string{
	"h":    " H",
	"j":    " J",
	"k":    " K",
	"l":    " L",
	"semi": "SC",
}

var laneDisplayNames = map[string]string{
	"h":    "H",
	"j":    "J",
	"k":    "K",
	"l":    "L",
	"semi": "SC",
}

type Session struct {
	ID   string
	Name string
}

type Window struct {
	ID        string
	Name      string
	Index     string
	Lane      string // "h", "j", "k", "l", "semi"
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
	// #{@lane} comes before #{window_name} so names with spaces are captured by SplitN.
	out, err := exec.Command("tmux", "list-windows", "-t", sessID, "-F",
		"#{window_id} #{window_index} #{@lane} #{window_name}").Output()
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
		lane := parts[2]
		// Validate non-empty lane values; unknown values default to "j".
		// Empty lane means explicitly unassigned — preserved as "".
		if lane != "" {
			valid := false
			for _, key := range laneOrder {
				if key == lane {
					valid = true
					break
				}
			}
			if !valid {
				lane = "j"
			}
		}
		windows = append(windows, Window{
			ID:        parts[0],
			Index:     parts[1],
			Lane:      lane,
			Name:      parts[3],
			SessionID: sessID,
		})
	}
	return windows, nil
}

// groupByLane groups windows into per-lane slices preserving tmux window order.
func groupByLane(windows []Window) map[string][]Window {
	lanes := make(map[string][]Window, len(laneOrder))
	for _, key := range laneOrder {
		lanes[key] = nil
	}
	for _, w := range windows {
		if w.Lane == "" {
			continue // unassigned windows are excluded from the grid
		}
		lanes[w.Lane] = append(lanes[w.Lane], w)
	}
	return lanes
}

func tmuxRun(args ...string) error {
	return exec.Command("tmux", args...).Run()
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

// getCurrentLane returns the @lane option of the currently active window.
func getCurrentLane() string {
	lane := tmuxGetCurrentWindowOption("@lane")
	if lane == "" {
		return "j"
	}
	return lane
}

// mostRecentWindowInLane returns the window in the given lane with the highest
// @hometown_visited timestamp. Falls back to the first window in the lane when
// none have a visit record. Returns nil if the lane has no windows.
func mostRecentWindowInLane(windows []Window, lane string) *Window {
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
func filterByLane(windows []Window, lane string) []Window {
	var result []Window
	for _, w := range windows {
		if w.Lane == lane {
			result = append(result, w)
		}
	}
	return result
}

// indexByID returns the index of the window with the given ID, or 0.
func indexByID(windows []Window, id string) int {
	for i, w := range windows {
		if w.ID == id {
			return i
		}
	}
	return 0
}

// shellSingleQuote wraps s in single quotes, escaping any single quotes within
// s using backslash. The result is safe to embed in a shell script regardless
// of what characters s contains.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
