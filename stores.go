package main

import (
	"fmt"
	"os/exec"
	"strings"
)

var storeKeys = []string{"h", "j", "k", "l", "semi"}

var storeDisplayNames = map[string]string{
	"h": "H", "j": "J", "k": "K", "l": "L", "semi": ";",
}

// storeSessionNames maps store keys to the name used when auto-creating a
// session for that slot. ";" becomes "SC" to keep the name shell-friendly.
var storeSessionNames = map[string]string{
	"h": "H", "j": "J", "k": "K", "l": "L", "semi": "SC",
}

// getSessionStoreKey returns the store key assigned to a session, or "".
func getSessionStoreKey(sessID string) string {
	return tmuxGetSessionOption(sessID, "@hometown_store_key")
}

// setSessionStoreKey assigns a session to a store key, or removes the
// assignment when key == "".
func setSessionStoreKey(sessID, key string) error {
	if key == "" {
		return exec.Command("tmux", "set-option", "-t", sessID, "-u", "@hometown_store_key").Run()
	}
	return exec.Command("tmux", "set-option", "-t", sessID, "@hometown_store_key", key).Run()
}

// getStoreSessions returns all sessions assigned to the given store key.
func getStoreSessions(key string) []Session {
	sessions, err := listAllSessions()
	if err != nil {
		return nil
	}
	var result []Session
	for _, s := range sessions {
		if getSessionStoreKey(s.ID) == key {
			result = append(result, s)
		}
	}
	return result
}

// groupByStore returns a map from store key to the sessions in that store.
func groupByStore() map[string][]Session {
	result := make(map[string][]Session)
	for _, key := range storeKeys {
		result[key] = nil
	}
	sessions, err := listAllSessions()
	if err != nil {
		return result
	}
	for _, s := range sessions {
		key := getSessionStoreKey(s.ID)
		if key != "" {
			result[key] = append(result[key], s)
		}
	}
	return result
}

// storeKeyForSession returns the store key for a session, or "".
func storeKeyForSession(sessID string) string {
	return getSessionStoreKey(sessID)
}

// setStore assigns a session to a store key.
func setStore(key, sessID string) error {
	return setSessionStoreKey(sessID, key)
}

// clearStoreForSession removes the store assignment from a session.
func clearStoreForSession(sessID string) error {
	return setSessionStoreKey(sessID, "")
}

// newStoreSession creates a new detached session for a store key, naming it
// "Session <X>" (where X comes from storeSessionNames) if that name is not
// already taken. The session's initial window is tagged with @lane "j".
func newStoreSession(key string) (string, error) {
	name := "Session " + storeSessionNames[key]
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
			exec.Command("tmux", "set-option", "-t", sessID, "@lane_j_window", winID).Run()
		}
	}
	return sessID, nil
}

// laneKeyToUserKey converts an internal lane/store key to the user-facing character.
func laneKeyToUserKey(key string) string {
	if key == "semi" {
		return ";"
	}
	return key
}

// parseStoreKey normalizes a user-provided key character to an internal key name.
func parseStoreKey(s string) (string, error) {
	switch strings.ToLower(s) {
	case ";", "semi", "semicolon", "sc":
		return "semi", nil
	}
	for _, k := range storeKeys {
		if s == k {
			return k, nil
		}
	}
	return "", fmt.Errorf("invalid key %q: must be h, j, k, l, or ;", s)
}
