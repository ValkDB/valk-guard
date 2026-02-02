// Package main is the entry point for claude-tidy, a TUI tool for managing
// Claude Code sessions.
package main

import (
	"fmt"
	"os"

	"claude-tidy/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	m := ui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
