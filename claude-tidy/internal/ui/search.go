package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
)

// newSearchInput creates a configured text input for search.
func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "search sessions..."
	ti.Prompt = "/ "
	ti.PromptStyle = searchPromptStyle
	ti.TextStyle = searchInputStyle
	ti.CharLimit = 100
	return ti
}
