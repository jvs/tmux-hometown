package main

import (
	"fmt"
	"os/exec"
	"strings"
)

var slotKeys = []string{"h", "j", "k", "l", "semi"}

var slotDisplayNames = map[string]string{
	"h": "H", "j": "J", "k": "K", "l": "L", "semi": ";",
}

// slotSessionNames maps slot keys to the name used when auto-creating a
// session for that slot. ";" becomes "SC" to keep the name shell-friendly.
var slotSessionNames = map[string]string{
	"h": "H", "j": "J", "k": "K", "l": "L", "semi": "SC",
}

// getSessionSlotKey returns the slot key assigned to a session, or "".
func getSessionSlotKey(sessID string) string {
	return tmuxGetSessionOption(sessID, "@hometown_slot_key")
}

// setSessionSlotKey assigns a session to a slot key, or removes the
// assignment when key == "".
func setSessionSlotKey(sessID, key string) error {
	if key == "" {
		return exec.Command("tmux", "set-option", "-t", sessID, "-u", "@hometown_slot_key").Run()
	}
	return exec.Command("tmux", "set-option", "-t", sessID, "@hometown_slot_key", key).Run()
}

// getSlotSessions returns all sessions assigned to the given slot key.
func getSlotSessions(key string) []Session {
	sessions, err := listAllSessions()
	if err != nil {
		return nil
	}
	var result []Session
	for _, s := range sessions {
		if getSessionSlotKey(s.ID) == key {
			result = append(result, s)
		}
	}
	return result
}

// groupBySlot returns a map from slot key to the sessions in that slot.
func groupBySlot() map[string][]Session {
	result := make(map[string][]Session)
	for _, key := range slotKeys {
		result[key] = nil
	}
	sessions, err := listAllSessions()
	if err != nil {
		return result
	}
	for _, s := range sessions {
		key := getSessionSlotKey(s.ID)
		if key != "" {
			result[key] = append(result[key], s)
		}
	}
	return result
}

// slotKeyForSession returns the slot key for a session, or "".
func slotKeyForSession(sessID string) string {
	return getSessionSlotKey(sessID)
}

// setSlot assigns a session to a slot key.
func setSlot(key, sessID string) error {
	return setSessionSlotKey(sessID, key)
}

// clearSlotForSession removes the slot assignment from a session.
func clearSlotForSession(sessID string) error {
	return setSessionSlotKey(sessID, "")
}

// newSlotSession creates a new detached session for a slot key, naming it
// "Session <X>" (where X comes from slotSessionNames) if that name is not
// already taken. The session's initial window is tagged with @lane "j".
func newSlotSession(key string) (string, error) {
	name := "Session " + slotSessionNames[key]
	out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-P", "-F", "#{session_id}").Output()
	if err != nil {
		// Name already taken — create without a specific name.
		out, err = exec.Command("tmux", "new-session", "-d", "-P", "-F", "#{session_id}").Output()
		if err != nil {
			return "", err
		}
	}
	sessID := strings.TrimSpace(string(out))
	// Tag the initial window as lane "j" and record it as that lane's window.
	winOut, err := exec.Command("tmux", "list-windows", "-t", sessID, "-F", "#{window_id}").Output()
	if err == nil {
		winID := strings.TrimSpace(string(winOut))
		if winID != "" {
			exec.Command("tmux", "set-window-option", "-t", winID, "@lane", "j").Run()
		}
	}
	return sessID, nil
}

// laneKeyToUserKey converts an internal lane/slot key to the user-facing character.
func laneKeyToUserKey(key string) string {
	if key == "semi" {
		return ";"
	}
	return key
}

// parseSlotKey normalizes a user-provided key character to an internal key name.
func parseSlotKey(s string) (string, error) {
	switch strings.ToLower(s) {
	case ";", "semi", "semicolon", "sc":
		return "semi", nil
	}
	for _, k := range slotKeys {
		if s == k {
			return k, nil
		}
	}
	return "", fmt.Errorf("invalid key %q: must be h, j, k, l, or ;", s)
}
