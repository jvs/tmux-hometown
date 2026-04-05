# tmux-hometown

A tmux window and session manager. Organizes windows into five lanes and sessions into five slots, displayed as popup grids you can navigate and edit without leaving your workflow.

## Installation

**With Go:**

```bash
go install github.com/jvs/tmux-hometown@latest
```

**Binary download (Go not required):**

```bash
# macOS (Apple Silicon)
curl -fsSL https://github.com/jvs/tmux-hometown/releases/latest/download/tmux-hometown-darwin-arm64 \
  -o ~/.local/bin/tmux-hometown && chmod +x ~/.local/bin/tmux-hometown

# macOS (Intel)
curl -fsSL https://github.com/jvs/tmux-hometown/releases/latest/download/tmux-hometown-darwin-amd64 \
  -o ~/.local/bin/tmux-hometown && chmod +x ~/.local/bin/tmux-hometown

# Linux (arm64)
curl -fsSL https://github.com/jvs/tmux-hometown/releases/latest/download/tmux-hometown-linux-arm64 \
  -o ~/.local/bin/tmux-hometown && chmod +x ~/.local/bin/tmux-hometown

# Linux (amd64)
curl -fsSL https://github.com/jvs/tmux-hometown/releases/latest/download/tmux-hometown-linux-amd64 \
  -o ~/.local/bin/tmux-hometown && chmod +x ~/.local/bin/tmux-hometown
```

Make sure `~/.local/bin` is on your `$PATH`.

## Configuration

Add bindings to your `tmux.conf` to open the popups:

```tmux
bind u run 'hometown show-windows'
bind U run 'hometown show-sessions'
```

That's enough to get started. The other popups (`show-grid`, `show-state`) are reachable from within the popup UI.

### Activation key

While any popup is open, the **activation key** (default: `u`) and **Tab** cycle forward through the popups. **Shift + activation key** and **Shift+Tab** cycle backward.

To use a different key, set the tmux option and update your bindings to match:

```tmux
set -g @hometown_activation_key y
bind y run 'hometown show-windows'
bind Y run 'hometown show-sessions'
```

### Cycle pattern

The order in which the activation key cycles through popups is controlled by `@hometown_cycle_pattern`. The default is:

```tmux
set -g @hometown_cycle_pattern 'state,grid,sessions,windows,history'
```

Forward cycling moves to the next name in the list, wrapping from the last back to the first. Backward cycling does the reverse. Any of the five popup names (`state`, `grid`, `sessions`, `windows`, `history`) can be reordered or omitted — omitting a name removes it from the cycle entirely. The current values of both options are visible in the `show-state` popup.

## Concepts

### Lanes and windows

Each tmux session is divided into five lanes — **H**, **J**, **K**, **L**, **;** — each holding its own set of windows. You switch lanes with `switch-window {h,j,k,l,;}` and cycle within a lane by pressing the same key again. Windows are tagged with the `@hometown_lane` tmux window option.

### Sessions and slots

Sessions are assigned to one of the same five slots — **H**, **J**, **K**, **L**, **;** — using the `@hometown_slot` session option. Multiple sessions can share a slot; `switch-session {key}` cycles through them. Sessions without a slot assignment are not shown in `show-sessions` but can be assigned one from within that popup.

## Popups

### show-windows

```
╭─ work ──────────────────────────────────────────────────────────────────────╮
│                                                                             │
│  H           J           K           L           ;                          │
│  ─────────────────────────────────────────────────────────────────────────  │
│  tests       editor      git         claude      scratch                    │
│              server                                                         │
│              staging                                                        │
│                                                                             │
│           [a]dd   [r]ename   [d]elete   [c]ut   [p]aste   re[m]ove          │
╰─────────────────────────────────────────────────────────────────────────────╯
```

Moving the cursor live-switches the active window so you can preview as you navigate.

#### Navigation

| Key | Action |
|-----|--------|
| `h` / `←` | Move left one lane |
| `l` / `→` | Move right one lane |
| `j` | Move down; wraps to the top of the next lane at the bottom |
| `k` | Move up; wraps to the bottom of the previous lane at the top |
| `↓` / `↑` | Move down / up within the lane (no wrap) |
| `alt`+lane key, or `shift`+lane key | Jump directly to that lane |

Lane keys are `h j k l ;` (plain or with alt/shift).

#### Actions

| Key | Action |
|-----|--------|
| `a` | Add a new window to the current lane |
| `r` | Rename the selected window |
| `d` | Kill the selected window (with confirmation) |
| `x` / `c` | Cut the selected window |
| `p` / `P` | Paste the cut window after / before the selected window |
| `m` | Remove the selected window from its lane (unassign) |
| `Enter` | Confirm and close; create a window if the lane is empty |
| `Esc` | Cancel and return to the original window |
| activation key / Tab | Cycle to next popup |
| shift + activation key / Shift+Tab | Cycle to previous popup |

#### Assigning untagged windows

If the current window has no lane assigned, show-windows opens with a prompt:

```
Assign a lane to window "my-window"?  [H] [J] [K] [L] [;]  [s]kip  [n]ever
```

- Pick a lane key to assign and proceed to the grid
- `s` — skip for now (prompt appears again next time)
- `n` — never prompt for this window again

---

### show-sessions

```
╭─ Hometown Sessions ─────────────────────────────────────────────────────────╮
│                                                                             │
│  H           J           K           L           ;                          │
│  ─────────────────────────────────────────────────────────────────────────  │
│  work        personal    server      -           -                          │
│              side-proj                                                      │
│                                                                             │
│           [a]dd   [r]ename   [d]elete   [c]ut   [p]aste   re[m]ove          │
╰─────────────────────────────────────────────────────────────────────────────╯
```

Moving the cursor live-switches the active session.

#### Navigation

| Key | Action |
|-----|--------|
| `h` / `←` | Move left one slot |
| `l` / `→` | Move right one slot |
| `j` | Move down; wraps to the top of the next slot at the bottom |
| `k` | Move up; wraps to the bottom of the previous slot at the top |
| `↓` / `↑` | Move down / up within the slot (no wrap) |
| `alt`+lane key, or `shift`+lane key | Jump directly to that slot |

#### Actions

| Key | Action |
|-----|--------|
| `a` | Create a new session in the current slot |
| `r` | Rename the selected session |
| `d` | Kill the selected session (with confirmation) |
| `x` / `c` | Cut the selected session |
| `p` | Paste the cut session into the current slot |
| `m` | Remove the selected session from its slot (unassign) |
| `Enter` | Confirm and close; create a session if the slot is empty |
| `Esc` | Cancel and return to the original session |
| `shift+enter` | Switch to show-windows |
| activation key / Tab | Cycle to next popup |
| shift + activation key / Shift+Tab | Cycle to previous popup |

Killing the last tmux session exits tmux. Otherwise the client is moved to a fallback session before the kill, so the popup survives.

#### Assigning unassigned sessions

If the current session has no slot assigned, show-sessions opens with a prompt:

```
Assign a slot to session "my-session"?  [H] [J] [K] [L] [;]  [s]kip  [n]ever
```

- Pick a slot key to assign and proceed to the grid
- `s` — skip for now (prompt appears again next time)
- `n` — never prompt for this session again

---

### show-grid

```
╭─ Hometown Grid ─────────────────────────────────────────────────────────────╮
│                                                                             │
│      Session           H           J           K           L           ;    │
│  ─────────────────────────────────────────────────────────────────────────  │
│  H   work              editor      server      git         claude      sc   │
│  J   personal          code        -           -           -           -    │
│  K   -                 -           -           -           -           -    │
│  L   -                 -           -           -           -           -    │
│  ;   -                 -           -           -           -           -    │
│                                                                             │
│           [a]dd   [r]ename   [d]elete   [c]ut   [p]aste   re[m]ove          │
╰─────────────────────────────────────────────────────────────────────────────╯
```

One row per session slot. Each window column shows the first window in that lane for the slot's session. The current row is shown in full brightness; other rows are dimmed. Moving the cursor live-switches to the selected session and window.

A `-` marks an empty cell. Pressing `Enter` on a `-` in the window columns creates the session and window in one step.

#### Navigation

| Key | Action |
|-----|--------|
| `h` / `←` | Move left one column |
| `l` / `→` | Move right one column |
| `j` / `↓` | Move down one row |
| `k` / `↑` | Move up one row |
| `alt`+lane key | Jump to that window column |
| `alt+shift`+lane key | Jump to that session row |

#### Actions

When the cursor is in the **Session** column, actions operate on the session. Otherwise they operate on the window.

| Key | Action |
|-----|--------|
| `a` | Add a new session (Session col) or window (window col) |
| `r` | Rename the selected session or window |
| `d` | Kill the selected session or window (with confirmation) |
| `x` / `c` | Cut the selected session or window |
| `p` | Paste the cut item into the current slot or lane |
| `m` | Remove the selected session from its slot, or window from its lane |
| `Enter` | Switch to the selected session/window and close; create if empty |
| `Esc` | Cancel and return to the original window |
| activation key / Tab | Cycle to next popup |
| shift + activation key / Shift+Tab | Cycle to previous popup |

## Commands

Running `hometown` with no arguments prints a help summary:

```
Usage: hometown <command> [args]

Navigation
  switch-window <h|j|k|l|;>   Switch to that lane; cycle within it, or create a new window
  switch-session <h|j|k|l|;>  Switch to that slot; cycle if multiple, or create a new session
  flip-window                  Toggle back to the previously active window in this session
  flip-session                 Toggle back to the previously active session
  new-window                   Create a new window in the current lane
  kill-window                  Kill the current window (with confirmation)
  kill-session                 Kill the current session (with confirmation)

History
  previous-window-in-current-session  Go to the most recently visited other window in this session
  next-window-in-current-session      Go forward in this session's window history
  previous-window-in-any-session      Go to the most recently visited other window across all sessions
  next-window-in-any-session          Go forward in the global window history
  previous-session                    Go to the most recently visited window in a different session
  next-session                        Go forward to the next session in history

Popups
  show-windows   Open (or close) the windows popup
  show-sessions  Open (or close) the sessions popup
  show-grid      Open (or close) the grid popup
  show-state     Open (or close) the state popup (debug view)

Utility
  record-visit [window-id]  Call from external tools (e.g. fzf switchers) when switching windows outside hometown, so history commands stay accurate
  version, --version        Show the current version
```

tmux-hometown tracks the last visit time of each window. The history commands navigate that ordering without recording new visits themselves.

All popups toggle: running the command while the popup is open closes it.

## Building

```bash
make build           # produces ./tmux-hometown (version: "dev")
make install         # go install (version: "dev")
```

## Releases

Binaries for macOS (arm64/amd64) and Linux (arm64/amd64) are built automatically
by GitHub Actions when a version tag is pushed. The version string is injected at
build time via ldflags — you never manually edit it in source.

To cut a release:

```bash
make release VERSION=v0.1.0
```

This will abort if you are not on `main` or if the working tree is dirty, then
push `main`, create the tag, and push the tag. The tag push triggers the release
workflow (`.github/workflows/release.yml`), which builds the binaries and
attaches them to a GitHub release.

### Versioning

This project uses [semantic versioning](https://semver.org/) with a `v` prefix.

- Stay on `v0.x.y` while the tool is evolving — it signals "works well but may change"
- Increment the **patch** version (`v0.1.0` → `v0.1.1`) for bug fixes
- Increment the **minor** version (`v0.1.0` → `v0.2.0`) for new features
- The **major** version stays at `0` until the interface is stable

Do not manually update `var version` in `main.go` or `VERSION` in the Makefile —
they default to `"dev"` intentionally. The git tag is the single source of truth.
