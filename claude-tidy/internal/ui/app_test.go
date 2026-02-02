package ui

import (
	"claude-tidy/internal/models"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	m := NewModel()

	if m.focusedPane != paneProjects {
		t.Error("initial focused pane should be projects")
	}
	if m.sortBy != sortByDate {
		t.Error("initial sort should be by date")
	}
	if m.state != stateBrowse {
		t.Error("initial state should be browse")
	}
	if m.projectIdx != 0 {
		t.Error("initial project index should be 0")
	}
	if m.sessionIdx != 0 {
		t.Error("initial session index should be 0")
	}
}

func TestModelUpdate_WindowSize(t *testing.T) {
	m := NewModel()
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}

	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.width != 120 {
		t.Errorf("width = %d, want 120", model.width)
	}
	if model.height != 40 {
		t.Errorf("height = %d, want 40", model.height)
	}
}

func TestModelUpdate_SessionsLoaded(t *testing.T) {
	m := NewModel()

	projects := []*models.Project{
		{
			Name:       "myproject",
			EncodedDir: "-Users-me-myproject",
			TotalSize:  1000,
			Sessions: []*models.Session{
				{
					ID:      "session-1",
					Size:    500,
					ModTime: time.Now(),
					Preview: "Test session",
				},
				{
					ID:      "session-2",
					Size:    500,
					ModTime: time.Now().Add(-time.Hour),
					Preview: "Another session",
				},
			},
		},
	}

	msg := sessionsLoadedMsg{projects: projects}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if len(model.projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(model.projects))
	}
	if model.totalDiskUsage != 1000 {
		t.Errorf("totalDiskUsage = %d, want 1000", model.totalDiskUsage)
	}
	if len(model.sessions) != 2 {
		t.Errorf("sessions = %d, want 2", len(model.sessions))
	}
}

func TestModelUpdate_SessionsLoadedError(t *testing.T) {
	m := NewModel()

	msg := sessionsLoadedMsg{err: os.ErrNotExist}
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.err == nil {
		t.Error("expected error to be set")
	}
}

func TestModelView_NoWidth(t *testing.T) {
	m := NewModel()
	view := m.View()
	if view != "Loading..." {
		t.Errorf("View() with zero width = %q, want %q", view, "Loading...")
	}
}

func TestSortSessions(t *testing.T) {
	now := time.Now()

	m := NewModel()
	m.sessions = []*models.Session{
		{ID: "a", Size: 100, ModTime: now.Add(-2 * time.Hour), Staleness: models.Fresh},
		{ID: "b", Size: 300, ModTime: now.Add(-1 * time.Hour), Staleness: models.Stale},
		{ID: "c", Size: 200, ModTime: now, Staleness: models.Aging},
	}

	// Sort by date (default - newest first)
	m.sortBy = sortByDate
	m.sortSessions()
	if m.sessions[0].ID != "c" {
		t.Errorf("sort by date: first = %q, want %q", m.sessions[0].ID, "c")
	}

	// Sort by size (largest first)
	m.sortBy = sortBySize
	m.sortSessions()
	if m.sessions[0].ID != "b" {
		t.Errorf("sort by size: first = %q, want %q", m.sessions[0].ID, "b")
	}

	// Sort by staleness (stalest first)
	m.sortBy = sortByStaleness
	m.sortSessions()
	if m.sessions[0].ID != "b" {
		t.Errorf("sort by staleness: first = %q, want %q", m.sessions[0].ID, "b")
	}
}

func TestModelNavigation(t *testing.T) {
	now := time.Now()

	m := NewModel()
	m.width = 120
	m.height = 40
	m.projects = []*models.Project{
		{
			Name:      "proj-a",
			TotalSize: 100,
			Sessions: []*models.Session{
				{ID: "s1", Preview: "Session 1", ModTime: now},
				{ID: "s2", Preview: "Session 2", ModTime: now},
			},
		},
		{
			Name:      "proj-b",
			TotalSize: 200,
			Sessions: []*models.Session{
				{ID: "s3", Preview: "Session 3", ModTime: now},
			},
		},
	}
	m.updateVisibleSessions()

	// Move down in projects
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	updated, _ := m.Update(msg)
	model := updated.(Model)
	if model.projectIdx != 1 {
		t.Errorf("after j: projectIdx = %d, want 1", model.projectIdx)
	}

	// Move back up
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.projectIdx != 0 {
		t.Errorf("after k: projectIdx = %d, want 0", model.projectIdx)
	}

	// Switch to sessions pane
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.focusedPane != paneSessions {
		t.Error("after l: should focus sessions pane")
	}

	// Switch back to projects
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.focusedPane != paneProjects {
		t.Error("after h: should focus projects pane")
	}
}

func TestSearchMode(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 40

	// Enter search mode
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	updated, _ := m.Update(msg)
	model := updated.(Model)
	if model.state != stateSearch {
		t.Error("after /: state should be search")
	}

	// Exit search mode
	msg = tea.KeyMsg{Type: tea.KeyEscape}
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.state != stateBrowse {
		t.Error("after esc: state should be browse")
	}
}

func TestDeleteConfirmation(t *testing.T) {
	now := time.Now()

	m := NewModel()
	m.width = 120
	m.height = 40
	m.focusedPane = paneSessions
	m.projects = []*models.Project{
		{
			Name: "proj",
			Sessions: []*models.Session{
				{ID: "s1", Preview: "Session 1", ModTime: now, Size: 100},
			},
		},
	}
	m.updateVisibleSessions()

	// Press d to trigger delete confirmation
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	updated, _ := m.Update(msg)
	model := updated.(Model)
	if model.state != stateConfirmDelete {
		t.Error("after d: state should be confirmDelete")
	}

	// Cancel with n
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.state != stateBrowse {
		t.Error("after n: state should be browse")
	}
}

func TestSortCycling(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 40

	if m.sortBy != sortByDate {
		t.Error("initial sort should be by date")
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}

	updated, _ := m.Update(msg)
	model := updated.(Model)
	if model.sortBy != sortBySize {
		t.Errorf("after first s: sortBy = %d, want sortBySize(%d)", model.sortBy, sortBySize)
	}

	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.sortBy != sortByStaleness {
		t.Errorf("after second s: sortBy = %d, want sortByStaleness(%d)", model.sortBy, sortByStaleness)
	}

	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.sortBy != sortByDate {
		t.Errorf("after third s: sortBy = %d, want sortByDate(%d)", model.sortBy, sortByDate)
	}
}
