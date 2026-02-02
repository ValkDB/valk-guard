package ui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"claude-tidy/internal/config"
	"claude-tidy/internal/models"
	"claude-tidy/internal/storage"
	"claude-tidy/internal/utils"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// appState represents the current UI state.
type appState int

const (
	stateBrowse appState = iota
	stateSearch
	stateNewSession
	stateConfirmDelete
)

// pane represents which pane is focused.
type pane int

const (
	paneProjects pane = iota
	paneSessions
)

// sortMode represents session sort order.
type sortMode int

const (
	sortByDate sortMode = iota
	sortBySize
	sortByStaleness
)

// Model is the main bubbletea model.
type Model struct {
	projects       []*models.Project
	sessions       []*models.Session // current filtered/visible sessions
	allSessions    []*models.Session // unfiltered sessions for current project
	projectIdx     int
	sessionIdx     int
	focusedPane    pane
	state          appState
	sortBy         sortMode
	totalDiskUsage int64
	width          int
	height         int
	searchInput    textinput.Model
	newModal       newSessionModal
	err            error

	// for exec-and-return
	execCmd *exec.Cmd
}

// sessionsLoadedMsg is sent after async session loading.
type sessionsLoadedMsg struct {
	projects []*models.Project
	err      error
}

// sessionDeletedMsg is sent after a session is deleted.
type sessionDeletedMsg struct {
	err error
}

// execFinishedMsg is sent when an external command finishes.
type execFinishedMsg struct {
	err error
}

// NewModel creates and initializes the app model.
func NewModel() Model {
	return Model{
		searchInput: newSearchInput(),
		newModal:    newNewSessionModal(),
		focusedPane: paneProjects,
		sortBy:      sortByDate,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return loadProjectsCmd()
}

func loadProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		projects, err := storage.LoadProjects()
		return sessionsLoadedMsg{projects: projects, err: err}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case sessionsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.projects = msg.projects
		m.totalDiskUsage = storage.TotalDiskUsage(m.projects)
		m.updateVisibleSessions()
		return m, nil

	case sessionDeletedMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.state = stateBrowse
		return m, loadProjectsCmd()

	case execFinishedMsg:
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	}

	// Handle text input updates
	var cmd tea.Cmd
	switch m.state {
	case stateSearch:
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applySearch()
		return m, cmd
	case stateNewSession:
		if m.newModal.activeField == fieldProject {
			m.newModal.projectInput, cmd = m.newModal.projectInput.Update(msg)
		} else {
			m.newModal.goalInput, cmd = m.newModal.goalInput.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global escape handling
	if key.Matches(msg, keys.Escape) {
		switch m.state {
		case stateSearch:
			m.state = stateBrowse
			m.searchInput.SetValue("")
			m.searchInput.Blur()
			m.updateVisibleSessions()
			return m, nil
		case stateNewSession:
			m.newModal.close()
			m.state = stateBrowse
			return m, nil
		case stateConfirmDelete:
			m.state = stateBrowse
			return m, nil
		}
	}

	switch m.state {
	case stateSearch:
		return m.handleSearchKeys(msg)
	case stateNewSession:
		return m.handleNewModalKeys(msg)
	case stateConfirmDelete:
		return m.handleConfirmKeys(msg)
	default:
		return m.handleBrowseKeys(msg)
	}
}

func (m Model) handleBrowseKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Up):
		if m.focusedPane == paneProjects {
			if m.projectIdx > 0 {
				m.projectIdx--
				m.sessionIdx = 0
				m.updateVisibleSessions()
			}
		} else {
			if m.sessionIdx > 0 {
				m.sessionIdx--
			}
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if m.focusedPane == paneProjects {
			if m.projectIdx < len(m.projects)-1 {
				m.projectIdx++
				m.sessionIdx = 0
				m.updateVisibleSessions()
			}
		} else {
			if m.sessionIdx < len(m.sessions)-1 {
				m.sessionIdx++
			}
		}
		return m, nil

	case key.Matches(msg, keys.Left):
		m.focusedPane = paneProjects
		return m, nil

	case key.Matches(msg, keys.Right), key.Matches(msg, keys.Tab):
		if len(m.sessions) > 0 {
			m.focusedPane = paneSessions
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		if m.focusedPane == paneSessions && m.sessionIdx < len(m.sessions) {
			return m, m.resumeSession()
		}
		if m.focusedPane == paneProjects && len(m.sessions) > 0 {
			m.focusedPane = paneSessions
		}
		return m, nil

	case key.Matches(msg, keys.Delete):
		if m.focusedPane == paneSessions && m.sessionIdx < len(m.sessions) {
			m.state = stateConfirmDelete
		}
		return m, nil

	case key.Matches(msg, keys.New):
		m.state = stateNewSession
		defaultProj := ""
		if m.projectIdx < len(m.projects) {
			defaultProj = storage.DecodeProjectPath(m.projects[m.projectIdx].EncodedDir)
		}
		m.newModal.open(defaultProj)
		return m, nil

	case key.Matches(msg, keys.Search):
		m.state = stateSearch
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		return m, m.searchInput.Focus()

	case key.Matches(msg, keys.Sort):
		m.sortBy = (m.sortBy + 1) % 3
		m.sortSessions()
		return m, nil

	case key.Matches(msg, keys.Refresh):
		return m, loadProjectsCmd()
	}

	return m, nil
}

func (m Model) handleSearchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Enter):
		// Finalize search, switch to browse with filtered results
		m.state = stateBrowse
		m.searchInput.Blur()
		m.focusedPane = paneSessions
		m.sessionIdx = 0
		return m, nil
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applySearch()
		return m, cmd
	}
}

func (m Model) handleNewModalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Tab):
		m.newModal.focusNext()
		return m, nil
	case key.Matches(msg, keys.Enter):
		return m, m.launchNewSession()
	default:
		var cmd tea.Cmd
		if m.newModal.activeField == fieldProject {
			m.newModal.projectInput, cmd = m.newModal.projectInput.Update(msg)
		} else {
			m.newModal.goalInput, cmd = m.newModal.goalInput.Update(msg)
		}
		return m, cmd
	}
}

func (m Model) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.sessionIdx < len(m.sessions) {
			sess := m.sessions[m.sessionIdx]
			return m, func() tea.Msg {
				err := storage.DeleteSession(sess)
				return sessionDeletedMsg{err: err}
			}
		}
		m.state = stateBrowse
		return m, nil
	default:
		m.state = stateBrowse
		return m, nil
	}
}

func (m *Model) updateVisibleSessions() {
	if m.projectIdx >= len(m.projects) {
		m.sessions = nil
		m.allSessions = nil
		return
	}
	m.allSessions = m.projects[m.projectIdx].Sessions
	m.sessions = m.allSessions
	m.sortSessions()
}

func (m *Model) applySearch() {
	query := m.searchInput.Value()
	if query == "" {
		m.sessions = m.allSessions
		return
	}

	m.sessions = storage.SearchSessions(m.allSessions, query)
	m.sessionIdx = 0
}

func (m *Model) sortSessions() {
	switch m.sortBy {
	case sortByDate:
		sort.Slice(m.sessions, func(i, j int) bool {
			return m.sessions[i].ModTime.After(m.sessions[j].ModTime)
		})
	case sortBySize:
		sort.Slice(m.sessions, func(i, j int) bool {
			return m.sessions[i].Size > m.sessions[j].Size
		})
	case sortByStaleness:
		sort.Slice(m.sessions, func(i, j int) bool {
			return m.sessions[i].Staleness > m.sessions[j].Staleness
		})
	}
}

func (m Model) resumeSession() tea.Cmd {
	sess := m.sessions[m.sessionIdx]
	c := exec.Command("claude", "--resume", sess.ID)
	c.Dir = sess.ProjectPath
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

func (m Model) launchNewSession() tea.Cmd {
	projectPath := m.newModal.projectInput.Value()
	goal := m.newModal.goalInput.Value()
	m.newModal.close()
	m.state = stateBrowse

	if goal != "" {
		// Save goal with a placeholder session ID - will be linked on next refresh
		_ = storage.SaveGoal("pending-"+fmt.Sprintf("%d", len(m.projects)), goal, projectPath)
	}

	c := exec.Command("claude")
	c.Dir = projectPath
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var sections []string

	// Header
	sections = append(sections, m.renderHeader())

	// Main content area
	mainHeight := m.height - 5 // header + footer
	if mainHeight < 5 {
		mainHeight = 5
	}

	switch m.state {
	case stateConfirmDelete:
		main := m.renderMainContent(mainHeight)
		// Overlay confirmation dialog
		if m.sessionIdx < len(m.sessions) {
			sess := m.sessions[m.sessionIdx]
			name := sess.Preview
			if sess.Goal != "" {
				name = sess.Goal
			}
			dialog := renderConfirmDialog(name, utils.FormatBytes(sess.Size), m.width)
			sections = append(sections, lipgloss.JoinVertical(lipgloss.Left, dialog, main))
		} else {
			sections = append(sections, main)
		}

	case stateNewSession:
		main := m.renderMainContent(mainHeight)
		dialog := m.newModal.render(m.width)
		sections = append(sections, lipgloss.JoinVertical(lipgloss.Left, dialog, main))

	default:
		sections = append(sections, m.renderMainContent(mainHeight))
	}

	// Search bar (if searching)
	if m.state == stateSearch {
		sections = append(sections, m.searchInput.View())
	}

	// Footer
	sections = append(sections, m.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderHeader() string {
	title := headerStyle.Render("◉ claude-tidy")
	usage := diskUsageStyle.Render(fmt.Sprintf("%s used", utils.FormatBytes(m.totalDiskUsage)))

	// Distribute across width
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(usage) - 4
	if gap < 1 {
		gap = 1
	}

	content := title + strings.Repeat(" ", gap) + usage
	return headerBoxStyle.Width(m.width - 2).Render(content)
}

func (m Model) renderMainContent(height int) string {
	leftWidth := m.width / 3
	if leftWidth < 20 {
		leftWidth = 20
	}
	rightWidth := m.width - leftWidth - 3 // 3 for divider padding

	left := renderProjectList(
		m.projects,
		m.projectIdx,
		leftWidth,
		height,
		m.focusedPane == paneProjects,
	)

	var projectName string
	var projectSize int64
	if m.projectIdx < len(m.projects) {
		projectName = m.projects[m.projectIdx].Name
		projectSize = m.projects[m.projectIdx].TotalSize
	}

	right := renderSessionList(
		m.sessions,
		m.sessionIdx,
		projectName,
		projectSize,
		rightWidth,
		height,
		m.focusedPane == paneSessions,
		m.state == stateSearch,
	)

	// Vertical divider
	divider := paneDividerStyle.Render(" │ ")

	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func (m Model) renderFooter() string {
	sep := footerDividerStyle.Render("  ")

	sortLabel := "date"
	switch m.sortBy {
	case sortBySize:
		sortLabel = "size"
	case sortByStaleness:
		sortLabel = "stale"
	}

	binds := []string{
		footerKeyStyle.Render("j/k") + " move",
		footerKeyStyle.Render("enter") + " resume",
		footerKeyStyle.Render("d") + " delete",
		footerKeyStyle.Render("n") + " new",
		footerKeyStyle.Render("/") + " search",
		footerKeyStyle.Render("s") + " sort:" + sortLabel,
		footerKeyStyle.Render("r") + " refresh",
		footerKeyStyle.Render("?") + " help",
	}

	left := footerStyle.Render(strings.Join(binds, sep))
	version := footerVersionStyle.Render("v" + config.AppVersion)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(version) - 2
	if gap < 1 {
		gap = 1
	}

	divider := footerDividerStyle.Render(strings.Repeat("─", m.width))

	footer := left + strings.Repeat(" ", gap) + version
	return lipgloss.JoinVertical(lipgloss.Left, divider, footer)
}
