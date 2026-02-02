# claude-tidy

A TUI (Terminal User Interface) tool for managing Claude Code sessions. Browse, search, clean up old conversations, and start new sessions with goals.

## Features

- **Session Browser** — Two-pane view: projects on the left, sessions on the right
- **Staleness Indicators** — Color-coded freshness: green (< 3 days), yellow (< 2 weeks), red (> 2 weeks)
- **Disk Usage** — See per-session and total storage consumption at a glance
- **Full-text Search** — Search across all session content with `/`
- **Session Goals** — Attach goals to new sessions for better organization
- **Resume/Delete** — Resume any session or delete stale ones to free disk space
- **Sortable** — Sort sessions by date, size, or staleness

## Requirements

- Go 1.24+
- Claude Code CLI (`claude`) installed for resume/new session features

## Installation

```bash
cd claude-tidy
go build -o claude-tidy .
```

Or install directly:

```bash
go install ./claude-tidy
```

## Usage

```bash
./claude-tidy
```

### Keybindings

| Key       | Action                    |
|-----------|---------------------------|
| `j` / `k` | Move up/down             |
| `h` / `l` | Switch panes             |
| `enter`   | Resume selected session   |
| `d`       | Delete session (confirm)  |
| `n`       | New session with goal     |
| `/`       | Search sessions           |
| `s`       | Cycle sort mode           |
| `r`       | Refresh                   |
| `q`       | Quit                      |

## Architecture

```
claude-tidy/
├── main.go                    # Entry point
├── internal/
│   ├── config/config.go       # Paths and constants
│   ├── models/                # Data structures
│   │   ├── project.go
│   │   └── session.go
│   ├── storage/               # Data access layer
│   │   ├── claude.go          # Read ~/.claude/ sessions (read-only)
│   │   └── goals.go           # Read/write ~/.claude-tidy/goals.json
│   ├── ui/                    # TUI components
│   │   ├── app.go             # Main bubbletea model
│   │   ├── styles.go          # Lipgloss styles (Catppuccin palette)
│   │   ├── keys.go            # Keybindings
│   │   ├── projects.go        # Project list renderer
│   │   ├── sessions.go        # Session card renderer
│   │   ├── search.go          # Search input
│   │   └── newmodal.go        # New session / confirm modals
│   └── utils/                 # Formatting helpers
│       ├── filesize.go
│       └── timeago.go
├── go.mod
└── go.sum
```

## Data Storage

- **Reads from** `~/.claude/projects/` — Session `.jsonl` files (never modified)
- **Writes to** `~/.claude-tidy/` — Goals and config (our own metadata)

## Design Principles

- **No network calls** — Runs 100% locally
- **Read-only on Claude's files** — Never modifies `~/.claude/`
- **Minimal dependencies** — Only Go stdlib + [Charm](https://charm.sh) stack

## License

Apache 2.0
