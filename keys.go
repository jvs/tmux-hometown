package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

const defaultLaneKeysStr = "hjkl;"
const defaultSlotKeysStr = "hjkl;"

// ── Control key resolution ────────────────────────────────────────────────────

// CtrlKey identifies one of the six popup control actions.
type CtrlKey int

const (
	CtrlAdd    CtrlKey = iota // add
	CtrlRename                // rename
	CtrlCut                   // cut
	CtrlPaste                 // paste
	CtrlHide                  // hide (remove from slot/lane without killing)
	CtrlKill                  // kill (delete)
)

// ctrlDef pairs a CtrlKey with its display label and ordered preference list.
type ctrlDef struct {
	key   CtrlKey
	label string
	prefs []string
}

// ctrlDefs lists the six controls in left-to-right resolution order.
// Edit the prefs slices here to change key preferences.
var ctrlDefs = []ctrlDef{
	{CtrlAdd, "add", []string{"a", "d", "w", "+", "1", "2", "3", "4", "5", "6", "7", "8", "9"}},
	{CtrlRename, "rename", []string{"r", "e", "n", "m", "1", "2", "3", "4", "5", "6", "7", "8", "9"}},
	{CtrlCut, "cut", []string{"x", "c", "u", "t", "y", "1", "2", "3", "4", "5", "6", "7", "8", "9"}},
	{CtrlPaste, "paste", []string{"p", "v", "s", "*", "1", "2", "3", "4", "5", "6", "7", "8", "9"}},
	{CtrlHide, "hide", []string{"i", "z", "g", "-", "1", "2", "3", "4", "5", "6", "7", "8", "9"}},
	{CtrlKill, "kill", []string{"q", "b", "f", "!", "1", "2", "3", "4", "5", "6", "7", "8", "9"}},
}

// resolvedCtrlKey maps each CtrlKey to its assigned keyboard key for the
// current hometown key config. Rebuilt by buildKeyState via buildCtrlState.
var resolvedCtrlKey map[CtrlKey]string

// resolvedCtrlFor is the reverse of resolvedCtrlKey: keyboard key → CtrlKey.
var resolvedCtrlFor map[string]CtrlKey

// resolvedHintBar is the pre-built hint string shown in popup control bars, e.g.
// "[a] add · [r] rename · [x] cut · [p] paste · [i] hide · [q] kill"
var resolvedHintBar string

// laneKeysError and slotKeysError are non-empty when the respective tmux
// options contain invalid values. Set by initKeys, read by popup views.
var laneKeysError string
var slotKeysError string

var keysErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

// initKeys reads @hometown_lane_keys and @hometown_slot_keys, validates each,
// and rebuilds all key-derived globals. Falls back to defaults on any error.
func initKeys() {
	rawLane := tmuxGetGlobalOption("@hometown_lane_keys")
	var laneRunes []rune
	if rawLane == "" {
		laneRunes = []rune(defaultLaneKeysStr)
		laneKeysError = ""
	} else {
		var err error
		laneRunes, err = validateKeys(rawLane)
		if err != nil {
			laneKeysError = fmt.Sprintf("@hometown_lane_keys %q: %v — using default %q", rawLane, err, defaultLaneKeysStr)
			laneRunes = []rune(defaultLaneKeysStr)
		} else {
			laneKeysError = ""
		}
	}

	rawSlot := tmuxGetGlobalOption("@hometown_slot_keys")
	var slotRunes []rune
	if rawSlot == "" {
		slotRunes = []rune(defaultSlotKeysStr)
		slotKeysError = ""
	} else {
		var err error
		slotRunes, err = validateKeys(rawSlot)
		if err != nil {
			slotKeysError = fmt.Sprintf("@hometown_slot_keys %q: %v — using default %q", rawSlot, err, defaultSlotKeysStr)
			slotRunes = []rune(defaultSlotKeysStr)
		} else {
			slotKeysError = ""
		}
	}

	buildKeyState(laneRunes, slotRunes)
}

// validateKeys ensures s contains exactly 5 unique printable non-space ASCII
// characters. Single-quote and backslash are excluded because they would
// break generated shell scripts.
func validateKeys(s string) ([]rune, error) {
	runes := []rune(s)
	if len(runes) != 5 {
		return nil, fmt.Errorf("must be exactly 5 characters, got %d", len(runes))
	}
	seen := map[rune]bool{}
	for _, r := range runes {
		if r < 33 || r > 126 {
			return nil, fmt.Errorf("character %q is not a printable ASCII character", r)
		}
		if r == '\'' || r == '\\' {
			return nil, fmt.Errorf("character %q is not allowed", r)
		}
		if seen[r] {
			return nil, fmt.Errorf("duplicate character %q", r)
		}
		seen[r] = true
	}
	return runes, nil
}

// buildKeyState initialises laneOrder, slotKeys, altLaneKey, and altSlotKey
// from validated rune slices, then resolves control keys.
func buildKeyState(laneK, slotK []rune) {
	laneOrder = make([]string, len(laneK))
	for i, r := range laneK {
		laneOrder[i] = string(r)
	}

	slotKeys = make([]string, len(slotK))
	for i, r := range slotK {
		slotKeys[i] = string(r)
	}

	// Each alt map holds both "alt+<key>" and "alt+<shift-variant>" entries
	// so that either form jumps to the same column.
	altLaneKey = make(map[string]int, len(laneK)*2)
	for i, r := range laneK {
		s := string(r)
		altLaneKey["alt+"+s] = i
		if sv := keyShiftVariant(r); sv != s {
			altLaneKey["alt+"+sv] = i
		}
	}

	altSlotKey = make(map[string]int, len(slotK)*2)
	for i, r := range slotK {
		s := string(r)
		altSlotKey["alt+"+s] = i
		if sv := keyShiftVariant(r); sv != s {
			altSlotKey["alt+"+sv] = i
		}
	}

	buildCtrlState(laneK, slotK)
}

// buildCtrlState resolves which keyboard key is assigned to each control
// action. Resolution goes left-to-right through ctrlDefs; each control claims
// the first preference not already taken by a lane key, slot key, or a
// previously resolved control.
func buildCtrlState(laneK, slotK []rune) {
	taken := make(map[string]bool, len(laneK)+len(slotK)+len(ctrlDefs))
	for _, r := range laneK {
		taken[string(r)] = true
	}
	for _, r := range slotK {
		taken[string(r)] = true
	}

	resolvedCtrlKey = make(map[CtrlKey]string, len(ctrlDefs))
	resolvedCtrlFor = make(map[string]CtrlKey, len(ctrlDefs))

	parts := make([]string, 0, len(ctrlDefs))
	for _, def := range ctrlDefs {
		var chosen string
		for _, pref := range def.prefs {
			if !taken[pref] {
				chosen = pref
				taken[pref] = true
				break
			}
		}
		if chosen == "" {
			chosen = "?" // shouldn't happen; digits ensure a fallback always exists
		}
		resolvedCtrlKey[def.key] = chosen
		resolvedCtrlFor[chosen] = def.key
		parts = append(parts, "["+chosen+"] "+def.label)
	}
	resolvedHintBar = strings.Join(parts, " · ")
}

// keyShiftVariant returns the shifted representation of r:
// lowercase letter → uppercase, ';' → ':', everything else → unchanged.
func keyShiftVariant(r rune) string {
	if r == ';' {
		return ":"
	}
	if unicode.IsLower(r) {
		return strings.ToUpper(string(r))
	}
	return string(r)
}

// ── Display helpers ───────────────────────────────────────────────────────────

// storeIndex returns the string to write into a tmux option for a lane/slot
// index. Using a plain digit avoids tmux treating ";" and other punctuation
// as command separators when the value is passed on the command line.
func storeIndex(i int) string { return strconv.Itoa(i) }

// parseIndex converts a stored index string back to an int.
// Returns -1 for any value that is not a valid index (including the empty
// string and old-format values like "h" or "semi").
func parseIndex(s string) int {
	if s == "" {
		return -1
	}
	i, err := strconv.Atoi(s)
	if err != nil || i < 0 || i >= len(laneOrder) {
		return -1
	}
	return i
}

// keyAtIndex returns keys[i], or "?" if i is out of range.
func keyAtIndex(keys []string, i int) string {
	if i < 0 || i >= len(keys) {
		return "?"
	}
	return keys[i]
}

// indexDisplay returns the UI label for lane position i.
func indexDisplay(i int) string { return keyDisplay(keyAtIndex(laneOrder, i)) }

// slotIndexDisplay returns the UI label for slot position i.
func slotIndexDisplay(i int) string { return keyDisplay(keyAtIndex(slotKeys, i)) }

// indexName returns a tmux-safe name fragment for lane position i.
func indexName(i int) string { return keyName(keyAtIndex(laneOrder, i)) }

// slotIndexName returns a tmux-safe name fragment for slot position i.
func slotIndexName(i int) string { return keyName(keyAtIndex(slotKeys, i)) }

// keyDisplay formats a key for display:
// - letters → uppercased  ("h" → "H")
// - digits  → as-is       ("1" → "1")
// - symbols → quoted      (";" → `";"`)
func keyDisplay(key string) string {
	r := firstRune(key)
	if unicode.IsLetter(r) {
		return strings.ToUpper(key)
	}
	if unicode.IsDigit(r) {
		return key
	}
	return `"` + key + `"`
}

// keyName returns a label for session/window names: letters are uppercased,
// ";" maps to "SC" (semicolons are not safe in tmux names), and everything
// else is left as-is.
func keyName(key string) string {
	if key == ";" {
		return "SC"
	}
	r := firstRune(key)
	if unicode.IsLetter(r) {
		return strings.ToUpper(key)
	}
	return key
}

// lanePromptKeyList builds the key portion of a lane assignment prompt,
// e.g. `[H]  [J]  [K]  [L]  [";"]`.
func lanePromptKeyList() string {
	parts := make([]string, len(laneOrder))
	for i := range laneOrder {
		parts[i] = "[" + indexDisplay(i) + "]"
	}
	return strings.Join(parts, "  ")
}

// slotPromptKeyList builds the key portion of a slot assignment prompt.
func slotPromptKeyList() string {
	parts := make([]string, len(slotKeys))
	for i := range slotKeys {
		parts[i] = "[" + slotIndexDisplay(i) + "]"
	}
	return strings.Join(parts, "  ")
}

// parseLaneKey validates that s is one of the currently configured lane key
// characters and returns its index.
func parseLaneKey(s string) (int, error) {
	for i, key := range laneOrder {
		if s == key {
			return i, nil
		}
	}
	displays := make([]string, len(laneOrder))
	for i, k := range laneOrder {
		displays[i] = keyDisplay(k)
	}
	return -1, fmt.Errorf("invalid key %q: must be one of %s", s, strings.Join(displays, ", "))
}

// keysErrorLine returns styled error banner line(s) for popup views,
// or "" when both key configs are valid.
func keysErrorLine(width int) string {
	var lines []string
	if laneKeysError != "" {
		lines = append(lines, keysErrorStyle.Width(width).Render("  ⚠  "+laneKeysError))
	}
	if slotKeysError != "" {
		lines = append(lines, keysErrorStyle.Width(width).Render("  ⚠  "+slotKeysError))
	}
	return strings.Join(lines, "\n")
}

// firstRune returns the first rune of s, or 0 if s is empty.
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}
