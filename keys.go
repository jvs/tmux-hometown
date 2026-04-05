package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

const defaultKeysStr = "hjkl;"

// keysError is non-empty when @hometown_keys contains an invalid value.
// It is set by initKeys and read by popup views to show an error banner.
var keysError string

var keysErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

// initKeys reads @hometown_keys, validates it, and rebuilds all key-derived
// globals. On any error it falls back to defaultKeysStr.
func initKeys() {
	raw := tmuxGetGlobalOption("@hometown_keys")
	var runes []rune
	if raw == "" {
		runes = []rune(defaultKeysStr)
		keysError = ""
	} else {
		var err error
		runes, err = validateKeys(raw)
		if err != nil {
			keysError = fmt.Sprintf("@hometown_keys %q: %v — using default %q", raw, err, defaultKeysStr)
			runes = []rune(defaultKeysStr)
		} else {
			keysError = ""
		}
	}
	buildKeyState(runes)
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

// buildKeyState initialises laneOrder, slotKeys, and the four popup key-maps
// from a validated rune slice.
func buildKeyState(keys []rune) {
	n := len(keys)
	laneOrder = make([]string, n)
	slotKeys = make([]string, n)
	for i, r := range keys {
		laneOrder[i] = string(r)
		slotKeys[i] = string(r)
	}

	laneKeyLane = make(map[string]int, n*4)
	laneKeyShift = make(map[string]bool, n*2)
	altLaneKeyLane = make(map[string]int, n)
	altShiftLaneKeyLane = make(map[string]int, n)

	for i, r := range keys {
		s := string(r)
		sv := keyShiftVariant(r)

		laneKeyLane[s] = i
		laneKeyShift[s] = false
		altLaneKeyLane["alt+"+s] = i

		if sv != s {
			laneKeyLane[sv] = i
			laneKeyShift[sv] = true
			altShiftLaneKeyLane["alt+"+sv] = i
		} else {
			// No distinct shift variant; alt+key doubles as the alt-shift entry.
			altShiftLaneKeyLane["alt+"+s] = i
		}
	}
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
// index.  Using a plain digit avoids tmux treating ";" and other punctuation
// as command separators when the value is passed on the command line.
func storeIndex(i int) string { return strconv.Itoa(i) }

// parseIndex converts a stored index string back to an int.
// Returns -1 for any value that is not a valid index into laneOrder
// (including the empty string and old-format values like "h" or "semi").
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

// indexDisplay returns the UI label for the lane/slot at position i.
func indexDisplay(i int) string {
	if i < 0 || i >= len(laneOrder) {
		return "?"
	}
	return keyDisplay(laneOrder[i])
}

// indexName returns a tmux-safe name fragment for the lane/slot at position i.
func indexName(i int) string {
	if i < 0 || i >= len(laneOrder) {
		return "?"
	}
	return keyName(laneOrder[i])
}

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

// promptKeyList builds the key portion of a lane/slot assignment prompt,
// e.g. `[H]  [J]  [K]  [L]  [";"]`.
func promptKeyList() string {
	parts := make([]string, len(laneOrder))
	for i := range laneOrder {
		parts[i] = "[" + indexDisplay(i) + "]"
	}
	return strings.Join(parts, "  ")
}

// keysErrorLine returns a full-width styled error banner for popup views,
// or "" when @hometown_keys is valid.
func keysErrorLine(width int) string {
	if keysError == "" {
		return ""
	}
	return keysErrorStyle.Width(width).Render("  ⚠  " + keysError)
}

// firstRune returns the first rune of s, or 0 if s is empty.
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}
