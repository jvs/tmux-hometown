# laneboard

A fast tmux lane visualizer. Shows all five lanes of the current session as
columns, with their windows listed beneath each heading. Designed to complement
the [lane system](https://github.com/jvs/dotfiles/blob/master/TMUX.md) in
`jvs/dotfiles`.

```
╭─ work ─────────────────────────────────────────────────────╮
│                                                            │
│                                                            │
│                                                            │
│  H           J           K           L           SC        │
│  ────────────────────────────────────────────────────────  │
│  tests       editor      git         claude      scratch   │
│              server                                        │
│              staging                                       │
│                                                            │
│        [a]dd   [r]ename   [d]elete   [c]ut   [p]aste       │
╰────────────────────────────────────────────────────────────╯
```

## Background

The lane system divides each tmux session into five semantic lanes — **H**
(tests), **J** (editor), **K** (git), **L** (Claude/zen), **;** (misc). Each
lane holds its own ring of windows and remembers which window was last focused.
You switch lanes with `alt+{h,j,k,l,;}` and cycle within a lane by pressing the
same key again.

laneboard gives you a bird's-eye view of the whole session at once, so you can
see every lane and every window without cycling through them one at a time.

## Usage

Open via `alt+u` (configured in `tmux-commands.zsh`). Press `alt+u` again to
close. Press `alt+o` to switch directly to supertree. Also available in the
command palette (`alt+y`) as "Open Laneboard" and in the menu (`alt+n`) as
"Open Laneboard".

## Navigation

Moving between lanes mirrors the lane keybindings you already use:

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` / `;` | Jump to that lane; or move **down** (wraps) if already there |
| `H` / `J` / `K` / `L` / `:` | Move **up** (wraps) within the current lane (shift variant) |
| `↑` / `↓` | Move up / down within the current lane |
| `←` / `→` | Move left / right between lanes |

Moving the cursor live-switches the underlying tmux window, so you can preview
windows as you navigate.

## Commands

| Key | Action |
|-----|--------|
| `a` | Add a new window to the current lane |
| `r` | Rename the selected window |
| `d` | Kill the selected window (with confirmation) |
| `x` or `c` | Mark the selected window for cut (shown with gray `(cut)` label) |
| `p` | Paste cut window **after** the selected window |
| `P` | Paste cut window **before** the selected window |
| `Enter` | Select and exit |
| `Esc` / `alt+u` | Cancel and return to the original window |
| `alt+o` | Switch to supertree |

### Adding windows

The new window is created immediately after the selected window in tmux's
window order and tagged with `@lane` so the lane system picks it up correctly.
When laneboard is running as a popup, the `new-window` command is written to a
file and executed after the popup closes, avoiding nested-popup issues.

### Killing windows

After a kill, focus moves to another window in the same lane if one exists,
otherwise to any other window in the session.

### Cut and paste

`x` (or `c`) marks the selected window for cut — it stays visible with a gray
`(cut)` label. Navigate to the destination lane and window, then press `p` to
paste after or `P` to paste before. The cut window's `@lane` tag is updated and
tmux's window order is adjusted so the position within the lane reflects the
paste location. The pasted window becomes selected.

## Implementation

Written in Go using [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lipgloss](https://github.com/charmbracelet/lipgloss), following the same
approach as [tmux-supertree](https://github.com/jvs/tmux-supertree).

Lane membership is read from the `@lane` tmux window option (values: `h`, `j`,
`k`, `l`, `semi`). Untagged windows default to lane-J, matching the lane
system's own orphan-adoption behavior.

The popup is sized dynamically: its height equals the maximum number of windows
in any single lane plus fixed overhead for the header, rule, padding, and hint
bar.

## Build

```
make
```

Produces the `laneboard` binary in the repo directory. The shell handler in
`tmux-commands.zsh` looks for it at `~/github/jvs/tmux-laneboard/laneboard` and
falls back to `$BIN_DIR/../runtime/tmux-laneboard/laneboard` for installed copies.
