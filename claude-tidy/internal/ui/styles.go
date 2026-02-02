package ui

import "github.com/charmbracelet/lipgloss"

// Catppuccin-inspired color palette
var (
	colorBase     = lipgloss.Color("#1e1e2e")
	colorSurface  = lipgloss.Color("#313244")
	colorOverlay  = lipgloss.Color("#45475a")
	colorText     = lipgloss.Color("#cdd6f4")
	colorSubtext  = lipgloss.Color("#a6adc8")
	colorLavender = lipgloss.Color("#b4befe")
	colorGreen    = lipgloss.Color("#a6e3a1")
	colorYellow   = lipgloss.Color("#f9e2af")
	colorRed      = lipgloss.Color("#f38ba8")
	colorPeach    = lipgloss.Color("#fab387")
	colorMauve    = lipgloss.Color("#cba6f7")
	colorTeal     = lipgloss.Color("#94e2d5")
)

// Header styles
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorLavender).
			PaddingLeft(1)

	headerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorOverlay).
			Foreground(colorText).
			PaddingLeft(1).
			PaddingRight(1)

	diskUsageStyle = lipgloss.NewStyle().
			Foreground(colorSubtext)
)

// Pane styles
var (
	paneTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorMauve).
			PaddingLeft(1).
			MarginBottom(1)

	paneActiveTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorLavender).
				PaddingLeft(1).
				MarginBottom(1)

	paneDividerStyle = lipgloss.NewStyle().
				Foreground(colorOverlay)
)

// Project list item styles
var (
	projectItemStyle = lipgloss.NewStyle().
				Foreground(colorText).
				PaddingLeft(2)

	projectSelectedStyle = lipgloss.NewStyle().
				Foreground(colorLavender).
				Bold(true).
				PaddingLeft(1)

	projectCountStyle = lipgloss.NewStyle().
				Foreground(colorSubtext)
)

// Session card styles
var (
	sessionCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorOverlay).
				Padding(0, 1).
				MarginBottom(0)

	sessionSelectedCardStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(colorLavender).
					Padding(0, 1).
					MarginBottom(0)

	sessionNameStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorText)

	sessionMetaStyle = lipgloss.NewStyle().
				Foreground(colorSubtext)

	sessionGoalStyle = lipgloss.NewStyle().
				Foreground(colorTeal)

	sessionWarningStyle = lipgloss.NewStyle().
				Foreground(colorRed).
				Bold(true)
)

// Staleness indicator styles
var (
	freshStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	agingStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	staleStyle = lipgloss.NewStyle().
			Foreground(colorRed)
)

// Footer styles
var (
	footerStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			PaddingLeft(1)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(colorLavender).
			Bold(true)

	footerDividerStyle = lipgloss.NewStyle().
				Foreground(colorOverlay)

	footerVersionStyle = lipgloss.NewStyle().
				Foreground(colorOverlay)
)

// Search styles
var (
	searchPromptStyle = lipgloss.NewStyle().
				Foreground(colorPeach).
				Bold(true)

	searchInputStyle = lipgloss.NewStyle().
				Foreground(colorText)

	searchMatchStyle = lipgloss.NewStyle().
				Foreground(colorPeach)
)

// Modal styles
var (
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMauve).
			Padding(1, 2).
			Width(50)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorMauve).
			MarginBottom(1)

	modalLabelStyle = lipgloss.NewStyle().
			Foreground(colorSubtext)

	modalHintStyle = lipgloss.NewStyle().
			Foreground(colorOverlay).
			Italic(true)
)

// Confirmation dialog styles
var (
	confirmStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorRed).
			Padding(1, 2).
			Width(50)

	confirmTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorRed)
)
