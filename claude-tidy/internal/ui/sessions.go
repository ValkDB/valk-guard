package ui

import (
	"fmt"
	"strings"

	"claude-tidy/internal/config"
	"claude-tidy/internal/models"
	"claude-tidy/internal/utils"

	"github.com/charmbracelet/lipgloss"
)

// stalenessIndicator returns a colored staleness icon.
func stalenessIndicator(s models.Staleness) string {
	switch s {
	case models.Fresh:
		return freshStyle.Render("●")
	case models.Aging:
		return agingStyle.Render("○")
	default:
		return staleStyle.Render("◌")
	}
}

// renderSessionList renders the right pane with session cards.
func renderSessionList(sessions []*models.Session, selected int, projectName string, projectSize int64, width, height int, focused bool, searching bool) string {
	var b strings.Builder

	// Title with project name and size
	titleText := fmt.Sprintf("SESSIONS › %s (%s)", projectName, utils.FormatBytes(projectSize))
	if focused {
		b.WriteString(paneActiveTitleStyle.Render(titleText))
	} else {
		b.WriteString(paneTitleStyle.Render(titleText))
	}
	b.WriteString("\n")

	divider := paneDividerStyle.Render(strings.Repeat("─", width-2))
	b.WriteString(divider)
	b.WriteString("\n")

	if len(sessions) == 0 {
		msg := "  No sessions found"
		if searching {
			msg = "  No matching sessions"
		}
		b.WriteString(sessionMetaStyle.Render(msg))
		return b.String()
	}

	cardWidth := width - 4
	if cardWidth < 20 {
		cardWidth = 20
	}

	linesUsed := 2
	for i, sess := range sessions {
		if linesUsed >= height-2 {
			break
		}

		card := renderSessionCard(sess, i == selected, cardWidth, searching)
		b.WriteString(card)
		b.WriteString("\n")
		linesUsed += 4 // approximate card height
	}

	return b.String()
}

// renderSessionCard renders a single session as a bordered card.
func renderSessionCard(s *models.Session, selected bool, width int, searching bool) string {
	var lines []string

	// Line 1: staleness indicator + name + optional warning
	indicator := stalenessIndicator(s.Staleness)
	name := s.Preview
	if s.Goal != "" {
		name = s.Goal
	}
	if len(name) > width-10 {
		name = name[:width-10] + "..."
	}

	nameLine := fmt.Sprintf("%s %s", indicator, sessionNameStyle.Render(name))

	// Add stale+large warning
	if s.Staleness == models.Stale && s.Size >= int64(config.LargeBytes) {
		nameLine += "  " + sessionWarningStyle.Render("⚠ stale")
	}
	lines = append(lines, nameLine)

	// Line 2: size + age
	meta := fmt.Sprintf("  %s · %s", utils.FormatBytes(s.Size), utils.TimeAgo(s.ModTime))
	if searching && s.MatchCount > 0 {
		meta += "  " + searchMatchStyle.Render(fmt.Sprintf("(%d matches)", s.MatchCount))
	}
	lines = append(lines, sessionMetaStyle.Render(meta))

	// Line 3: goal or preview quote
	if s.Goal != "" {
		goalLine := sessionGoalStyle.Render(fmt.Sprintf("  🎯 %s", s.Goal))
		lines = append(lines, goalLine)
	} else if s.Preview != "" && s.Preview != "(no preview)" {
		previewLine := sessionMetaStyle.Render(fmt.Sprintf("  \"%s\"", s.Preview))
		lines = append(lines, previewLine)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	style := sessionCardStyle.Width(width)
	if selected {
		style = sessionSelectedCardStyle.Width(width)
	}

	return style.Render(content)
}
