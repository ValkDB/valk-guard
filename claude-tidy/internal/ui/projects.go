package ui

import (
	"fmt"
	"strings"

	"claude-tidy/internal/models"
	"claude-tidy/internal/utils"

	"github.com/charmbracelet/lipgloss"
)

// renderProjectList renders the left pane with project list.
func renderProjectList(projects []*models.Project, selected int, width, height int, focused bool) string {
	var b strings.Builder

	title := "PROJECTS"
	if focused {
		b.WriteString(paneActiveTitleStyle.Render(title))
	} else {
		b.WriteString(paneTitleStyle.Render(title))
	}
	b.WriteString("\n")

	divider := paneDividerStyle.Render(strings.Repeat("─", width-2))
	b.WriteString(divider)
	b.WriteString("\n")

	if len(projects) == 0 {
		b.WriteString(sessionMetaStyle.Render("  No projects found"))
		b.WriteString("\n")
		b.WriteString(sessionMetaStyle.Render("  ~/.claude/projects/"))
		return b.String()
	}

	linesUsed := 2 // title + divider
	for i, proj := range projects {
		if linesUsed >= height-1 {
			break
		}

		count := fmt.Sprintf("%d", len(proj.Sessions))
		sizeStr := utils.FormatBytes(proj.TotalSize)

		var line string
		if i == selected {
			name := projectSelectedStyle.Render("› " + proj.Name)
			meta := projectCountStyle.Render(fmt.Sprintf(" %s  %s", count, sizeStr))
			line = lipgloss.JoinHorizontal(lipgloss.Left, name, meta)
		} else {
			name := projectItemStyle.Render("  " + proj.Name)
			meta := projectCountStyle.Render(fmt.Sprintf(" %s", count))
			line = lipgloss.JoinHorizontal(lipgloss.Left, name, meta)
		}

		b.WriteString(line)
		b.WriteString("\n")
		linesUsed++
	}

	return b.String()
}
