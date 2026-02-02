package models

import "time"

// Staleness levels for sessions
type Staleness int

const (
	Fresh  Staleness = iota // < 3 days
	Aging                   // < 2 weeks
	Stale                   // > 2 weeks
)

// Session represents a Claude Code conversation session.
type Session struct {
	ID          string    // UUID from filename
	FilePath    string    // Full path to .jsonl file
	ProjectPath string    // Decoded project path
	ProjectKey  string    // Encoded folder name
	Size        int64     // File size in bytes
	ModTime     time.Time // Last modification time
	Preview     string    // First user message (truncated)
	Goal        string    // User-set goal (from goals.json)
	Staleness   Staleness // Freshness indicator
	MatchCount  int       // Search match count (when searching)
}
