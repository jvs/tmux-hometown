package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// slotKeys contains the configured slot keys in order, e.g. ["h","j","k","l",";"].
// Populated by initKeys() at startup via buildKeyState.
var slotKeys []string

// getSessionSlotKey returns the slot index assigned to a session, and whether
// one is set.  The stored tmux option is a digit string ("0"–"4").
func getSessionSlotKey(sessID string) (int, bool) {
	i := parseIndex(tmuxGetSessionOption(sessID, "@hometown_slot"))
	if i < 0 {
		return 0, false
	}
	return i, true
}

// setSessionSlotKey records slot index i on a session.
func setSessionSlotKey(sessID string, i int) error {
	return exec.Command("tmux", "set-option", "-t", sessID, "@hometown_slot", storeIndex(i)).Run()
}

// clearSlotForSession removes the slot assignment from a session.
func clearSlotForSession(sessID string) error {
	return exec.Command("tmux", "set-option", "-t", sessID, "-u", "@hometown_slot").Run()
}

// getSlotSessions returns all sessions assigned to slot index i.
func getSlotSessions(i int) []Session {
	sessions, err := listAllSessions()
	if err != nil {
		return nil
	}
	var result []Session
	for _, s := range sessions {
		si, ok := getSessionSlotKey(s.ID)
		if ok && si == i {
			result = append(result, s)
		}
	}
	return result
}

// groupBySlot returns a map from slot index to the sessions in that slot.
func groupBySlot() map[int][]Session {
	result := make(map[int][]Session, len(slotKeys))
	for i := range slotKeys {
		result[i] = nil
	}
	sessions, err := listAllSessions()
	if err != nil {
		return result
	}
	for _, s := range sessions {
		i, ok := getSessionSlotKey(s.ID)
		if ok {
			result[i] = append(result[i], s)
		}
	}
	return result
}

// setSlot assigns session sessID to slot index i.
func setSlot(i int, sessID string) error {
	return setSessionSlotKey(sessID, i)
}

// newSlotSession creates a new detached session for slot index i, naming it
// "Session <X>" if that name is not already taken.  The session's initial
// window is tagged with lane index 1 (the default lane).
func newSlotSession(i int) (string, error) {
	name := "Session " + slotIndexName(i)
	out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-P", "-F", "#{session_id}").Output()
	if err != nil {
		// Name already taken — create without a specific name.
		out, err = exec.Command("tmux", "new-session", "-d", "-P", "-F", "#{session_id}").Output()
		if err != nil {
			return "", err
		}
	}
	sessID := strings.TrimSpace(string(out))
	winOut, err := exec.Command("tmux", "list-windows", "-t", sessID, "-F", "#{window_id}").Output()
	if err == nil {
		winID := strings.TrimSpace(string(winOut))
		if winID != "" {
			exec.Command("tmux", "set-window-option", "-t", winID, "@hometown_lane", storeIndex(1)).Run()
		}
	}
	return sessID, nil
}

// parseSlotKey validates that s is one of the currently configured slot key
// characters and returns its index.
func parseSlotKey(s string) (int, error) {
	for i, key := range slotKeys {
		if s == key {
			return i, nil
		}
	}
	displays := make([]string, len(slotKeys))
	for i, k := range slotKeys {
		displays[i] = keyDisplay(k)
	}
	return -1, fmt.Errorf("invalid key %q: must be one of %s", s, strings.Join(displays, ", "))
}
