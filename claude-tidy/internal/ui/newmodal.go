package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

type modalField int

const (
	fieldProject modalField = iota
	fieldGoal
)

type newSessionModal struct {
	projectInput textinput.Model
	goalInput    textinput.Model
	activeField  modalField
	visible      bool
}

func newNewSessionModal() newSessionModal {
	pi := textinput.New()
	pi.Placeholder = "/path/to/project"
	pi.Prompt = "  "
	pi.CharLimit = 200

	gi := textinput.New()
	gi.Placeholder = "e.g. Fix N+1 queries in dashboard"
	gi.Prompt = "  "
	gi.CharLimit = 200

	return newSessionModal{
		projectInput: pi,
		goalInput:    gi,
	}
}

func (m *newSessionModal) open(defaultProject string) {
	m.visible = true
	m.activeField = fieldProject
	m.projectInput.SetValue(defaultProject)
	m.goalInput.SetValue("")
	m.projectInput.Focus()
}

func (m *newSessionModal) close() {
	m.visible = false
	m.projectInput.Blur()
	m.goalInput.Blur()
}

func (m *newSessionModal) focusNext() {
	if m.activeField == fieldProject {
		m.activeField = fieldGoal
		m.projectInput.Blur()
		m.goalInput.Focus()
	} else {
		m.activeField = fieldProject
		m.goalInput.Blur()
		m.projectInput.Focus()
	}
}

func (m *newSessionModal) render(width int) string {
	if !m.visible {
		return ""
	}

	modalWidth := 50
	if width < 55 {
		modalWidth = width - 5
	}

	var lines []string
	lines = append(lines, modalTitleStyle.Render("New Session"))
	lines = append(lines, "")
	lines = append(lines, modalLabelStyle.Render("Project directory:"))
	lines = append(lines, m.projectInput.View())
	lines = append(lines, "")
	lines = append(lines, modalLabelStyle.Render("Goal:"))
	lines = append(lines, m.goalInput.View())
	lines = append(lines, "")
	lines = append(lines, modalHintStyle.Render("tab: switch field  enter: confirm  esc: cancel"))

	content := strings.Join(lines, "\n")
	box := modalStyle.Width(modalWidth).Render(content)

	// Center the modal
	return lipgloss.Place(width, 0, lipgloss.Center, lipgloss.Top, box)
}

// renderConfirmDialog renders a delete confirmation dialog.
func renderConfirmDialog(sessionName string, sessionSize string, width int) string {
	modalWidth := 50
	if width < 55 {
		modalWidth = width - 5
	}

	var lines []string
	lines = append(lines, confirmTitleStyle.Render("Delete Session"))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Are you sure you want to delete:"))
	lines = append(lines, sessionNameStyle.Render("  "+sessionName))
	lines = append(lines, sessionMetaStyle.Render("  "+sessionSize))
	lines = append(lines, "")
	lines = append(lines, modalHintStyle.Render("y: confirm  n/esc: cancel"))

	content := strings.Join(lines, "\n")
	box := confirmStyle.Width(modalWidth).Render(content)

	return lipgloss.Place(width, 0, lipgloss.Center, lipgloss.Top, box)
}
