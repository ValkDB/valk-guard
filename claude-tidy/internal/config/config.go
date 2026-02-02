package config

import (
	"os"
	"path/filepath"
)

const (
	AppName    = "claude-tidy"
	AppVersion = "0.1.0"

	// Staleness thresholds
	FreshDays  = 3
	StaleDays  = 14
	LargeBytes = 1 << 30 // 1 GB
)

// ClaudeDir returns the path to ~/.claude/
func ClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// ProjectsDir returns the path to ~/.claude/projects/
func ProjectsDir() string {
	return filepath.Join(ClaudeDir(), "projects")
}

// TidyDir returns the path to ~/.claude-tidy/
func TidyDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude-tidy")
}

// GoalsFile returns the path to ~/.claude-tidy/goals.json
func GoalsFile() string {
	return filepath.Join(TidyDir(), "goals.json")
}

// EnsureTidyDir creates ~/.claude-tidy/ if it doesn't exist
func EnsureTidyDir() error {
	return os.MkdirAll(TidyDir(), 0755)
}
