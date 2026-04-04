# tmux-hometown

A tmux window and session manager. Organizes windows into five lanes and sessions into five slots, displayed as popup grids you can navigate and edit without leaving your workflow.

## Concepts

### Lanes and windows

Each tmux session is divided into five lanes — **H**, **J**, **K**, **L**, **;** — each holding its own set of windows. You switch lanes with `switch-window {h,j,k,l,;}` and cycle within a lane by pressing the same key again. Windows are tagged with the `@lane` tmux option.

### Sessions

Sessions are assigned to one of the same five slots — **H**, **J**, **K**, **L**, **;** — using the `@hometown_store_key` session option. Multiple sessions can share a slot; `switch-session {key}` cycles through them. Sessions without a slot assignment are not shown in `show-sessions` but can be assigned one from that view.

## Commands

### Navigation

| Command | Description |
|---------|-------------|
| `switch-window {h,j,k,l,;}` | Switch to that lane's last window; cycle within the lane if already there; create a window if the lane is empty |
| `switch-session {h,j,k,l,;}` | Switch to the session assigned to that slot; cycle if multiple; create one if empty |
| `flip-window` | Toggle back to the previously active lane |
| `flip-session` | Toggle back to the previously active session slot |
| `new-window` | Create a new window in the current lane |
| `kill-window` | Kill the current window (with tmux confirmation prompt) |
| `kill-session` | Kill the current session (with tmux confirmation prompt) |

### History navigation

tmux-hometown tracks the last visit time of each window. The history commands navigate that ordering without recording new visits themselves.

| Command | Description |
|---------|-------------|
| `previous-window-in-current-session` | Go to the most recently visited other window in this session |
| `next-window-in-current-session` | Go forward in this session's window history |
| `previous-window-in-any-session` | Go to the most recently visited other window across all sessions |
| `next-window-in-any-session` | Go forward in the global window history |
| `previous-session` | Go to the most recently visited window in a different session |
| `next-session` | Go forward to the next session in history |

### Popups

| Command | Description |
|---------|-------------|
| `show-windows` | Open (or close) the windows popup |
| `show-sessions` | Open (or close) the sessions popup |
| `show-all` | Open (or close) the Hometown Sessions popup |

All three popups toggle: running the command while the popup is open closes it.

## show-windows

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

### Navigation

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` / `;` | Jump to that lane; cycle down within it if already there |
| `↑` / `↓` | Move up / down within the current lane |
| `←` / `→` | Move left / right between lanes |

### Actions

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
| `u` / `U` | Switch to show-sessions |

Shift+lane key (`H`, `J`, `K`, `L`, `:`) switches to that session slot and opens show-windows for it.

### Assigning untagged windows

If the current window has no lane assigned, show-windows opens with a prompt:

```
Assign a key to window "my-window"?  [H] [J] [K] [L] [;]  [s]kip  [n]ever
```

- Pick a lane key to assign and proceed to the grid
- `s` — skip for now (prompt appears again next time)
- `n` — never prompt for this window again

## show-sessions

```
╭─ Sessions ──────────────────────────────────────────────────────────────────╮
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

### Navigation

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` / `;` | Jump to that slot; cycle down within it if already there |
| `↑` / `↓` | Move up / down within the current slot |
| `←` / `→` | Move left / right between slots |

### Actions

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
| `u` / `U` / `shift+enter` | Switch to show-windows |

Shift+slot key (`H`, `J`, `K`, `L`, `:`) switches to that session slot and opens show-windows for it.

Killing the last tmux session exits tmux. Otherwise the client is moved to a fallback session before the kill, so the popup survives.

### Assigning unassigned sessions

If the current session has no slot assigned, show-sessions opens with a prompt:

```
Assign a key to session "my-session"?  [H] [J] [K] [L] [;]  [s]kip  [n]ever
```

- Pick a slot key to assign and proceed to the grid
- `s` — skip for now (prompt appears again next time)
- `n` — never prompt for this session again

## show-all (Hometown Sessions)

```
╭─ Hometown Sessions ─────────────────────────────────────────────────────────╮
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

### Navigation

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` / `;` | Jump to that window column |
| `H` / `J` / `K` / `L` / `:` | Jump to that session row |
| `↑` / `↓` | Move up / down one row |
| `←` / `→` | Move left / right one column |

### Actions

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

## Install

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

## Build

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
