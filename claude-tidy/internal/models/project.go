package models

// Project represents a Claude Code project directory.
type Project struct {
	Name       string     // Human-readable decoded path
	EncodedDir string     // Folder name in ~/.claude/projects/
	Sessions   []*Session // Sessions belonging to this project
	TotalSize  int64      // Sum of all session sizes
}
