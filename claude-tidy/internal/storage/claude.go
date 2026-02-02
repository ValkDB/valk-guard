package storage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"claude-tidy/internal/config"
	"claude-tidy/internal/models"
)

// DecodeProjectPath converts encoded folder name to a filesystem path.
// e.g. "-Users-me-project" -> "/Users/me/project"
func DecodeProjectPath(encoded string) string {
	if len(encoded) == 0 {
		return ""
	}
	// The leading dash represents the root slash
	// Subsequent dashes represent path separators
	return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
}

// ProjectDisplayName returns the last component of the decoded path.
func ProjectDisplayName(encoded string) string {
	decoded := DecodeProjectPath(encoded)
	return filepath.Base(decoded)
}

// LoadProjects scans ~/.claude/projects/ and returns all projects with sessions.
func LoadProjects() ([]*models.Project, error) {
	projectsDir := config.ProjectsDir()

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	goals, _ := LoadGoals()

	var projects []*models.Project
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		proj := &models.Project{
			Name:       ProjectDisplayName(entry.Name()),
			EncodedDir: entry.Name(),
		}

		sessionDir := filepath.Join(projectsDir, entry.Name())
		sessions, err := loadSessions(sessionDir, entry.Name(), goals)
		if err != nil {
			continue
		}

		proj.Sessions = sessions
		for _, s := range sessions {
			proj.TotalSize += s.Size
		}

		if len(proj.Sessions) > 0 {
			projects = append(projects, proj)
		}
	}

	// Sort projects by name
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return projects, nil
}

// loadSessions reads all .jsonl files in a project directory.
func loadSessions(dir, encodedDir string, goals *GoalsData) ([]*models.Session, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []*models.Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")

		s := &models.Session{
			ID:          sessionID,
			FilePath:    filePath,
			ProjectPath: DecodeProjectPath(encodedDir),
			ProjectKey:  encodedDir,
			Size:        info.Size(),
			ModTime:     info.ModTime(),
			Staleness:   computeStaleness(info.ModTime()),
		}

		// Check for a goal
		if goals != nil {
			if sg, ok := goals.Sessions[sessionID]; ok {
				s.Goal = sg.Goal
			}
		}

		// Extract preview from first user message
		s.Preview = extractPreview(filePath)

		sessions = append(sessions, s)
	}

	// Sort by modification time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	return sessions, nil
}

// computeStaleness determines freshness based on modification time.
func computeStaleness(modTime time.Time) models.Staleness {
	age := time.Since(modTime)
	switch {
	case age < time.Duration(config.FreshDays)*24*time.Hour:
		return models.Fresh
	case age < time.Duration(config.StaleDays)*24*time.Hour:
		return models.Aging
	default:
		return models.Stale
	}
}

// extractPreview reads the first user message from a .jsonl file.
func extractPreview(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer for potentially large JSON lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	linesRead := 0
	for scanner.Scan() {
		linesRead++
		if linesRead > 50 {
			break
		}

		line := scanner.Text()
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		// Check for user/human message types
		msgType, _ := msg["type"].(string)
		role, _ := msg["role"].(string)

		if msgType == "human" || msgType == "user" || role == "human" || role == "user" {
			// Try "message" field first, then "content"
			if text, ok := msg["message"].(string); ok && text != "" {
				return truncate(text, 60)
			}
			if text, ok := msg["content"].(string); ok && text != "" {
				return truncate(text, 60)
			}
			// content might be an array
			if content, ok := msg["content"].([]interface{}); ok {
				for _, c := range content {
					if cm, ok := c.(map[string]interface{}); ok {
						if text, ok := cm["text"].(string); ok && text != "" {
							return truncate(text, 60)
						}
					}
				}
			}
		}
	}

	return "(no preview)"
}

func truncate(s string, max int) string {
	// Remove newlines for display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// SearchSessions searches within session .jsonl files for a query string.
func SearchSessions(sessions []*models.Session, query string) []*models.Session {
	if query == "" {
		return sessions
	}

	query = strings.ToLower(query)
	var results []*models.Session

	for _, s := range sessions {
		count := searchInFile(s.FilePath, query)
		if count > 0 {
			s.MatchCount = count
			results = append(results, s)
		}
	}

	return results
}

func searchInFile(path, query string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	matches := 0
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		if strings.Contains(line, query) {
			matches++
		}
	}
	return matches
}

// DeleteSession removes a session .jsonl file.
func DeleteSession(session *models.Session) error {
	return os.Remove(session.FilePath)
}

// TotalDiskUsage returns the sum of all session file sizes.
func TotalDiskUsage(projects []*models.Project) int64 {
	var total int64
	for _, p := range projects {
		total += p.TotalSize
	}
	return total
}
