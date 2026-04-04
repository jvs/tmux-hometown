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
		if lane == "" {
			lane = "j" // unassigned windows default to lane-j
		}
		// Validate lane; anything unknown goes to j.
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
		lanes[w.Lane] = append(lanes[w.Lane], w)
	}
	return lanes
}

func tmuxRun(args ...string) error {
	return exec.Command("tmux", args...).Run()
}
