package storage

import (
	"claude-tidy/internal/models"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeProjectPath(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		want    string
	}{
		{"standard path", "-Users-me-project", "/Users/me/project"},
		{"deep path", "-Users-me-code-myapp", "/Users/me/code/myapp"},
		{"root-level", "-tmp", "/tmp"},
		{"home only", "-home-user", "/home/user"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeProjectPath(tt.encoded)
			if got != tt.want {
				t.Errorf("DecodeProjectPath(%q) = %q, want %q", tt.encoded, got, tt.want)
			}
		})
	}
}

func TestProjectDisplayName(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		want    string
	}{
		{"standard", "-Users-me-project", "project"},
		{"deep path", "-Users-me-code-myapp", "myapp"},
		{"single", "-tmp", "tmp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectDisplayName(tt.encoded)
			if got != tt.want {
				t.Errorf("ProjectDisplayName(%q) = %q, want %q", tt.encoded, got, tt.want)
			}
		})
	}
}

func TestExtractPreview(t *testing.T) {
	// Create a temp .jsonl file with test data
	dir := t.TempDir()

	t.Run("user message with message field", func(t *testing.T) {
		path := filepath.Join(dir, "session1.jsonl")
		lines := []map[string]interface{}{
			{"type": "system", "message": "System init"},
			{"type": "human", "message": "Fix the login bug in the auth module"},
		}
		writeJSONL(t, path, lines)

		got := extractPreview(path)
		if got != "Fix the login bug in the auth module" {
			t.Errorf("extractPreview() = %q, want %q", got, "Fix the login bug in the auth module")
		}
	})

	t.Run("user message with role field", func(t *testing.T) {
		path := filepath.Join(dir, "session2.jsonl")
		lines := []map[string]interface{}{
			{"role": "system", "content": "System init"},
			{"role": "user", "content": "Refactor the database layer"},
		}
		writeJSONL(t, path, lines)

		got := extractPreview(path)
		if got != "Refactor the database layer" {
			t.Errorf("extractPreview() = %q, want %q", got, "Refactor the database layer")
		}
	})

	t.Run("truncation", func(t *testing.T) {
		path := filepath.Join(dir, "session3.jsonl")
		longMsg := "This is a very long message that should definitely be truncated because it exceeds the maximum character limit"
		lines := []map[string]interface{}{
			{"type": "human", "message": longMsg},
		}
		writeJSONL(t, path, lines)

		got := extractPreview(path)
		if len(got) > 65 { // 60 + "..."
			t.Errorf("extractPreview() not truncated, len = %d", len(got))
		}
	})

	t.Run("no user message", func(t *testing.T) {
		path := filepath.Join(dir, "session4.jsonl")
		lines := []map[string]interface{}{
			{"type": "system", "message": "System init"},
			{"type": "assistant", "message": "Hello!"},
		}
		writeJSONL(t, path, lines)

		got := extractPreview(path)
		if got != "(no preview)" {
			t.Errorf("extractPreview() = %q, want %q", got, "(no preview)")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		got := extractPreview(filepath.Join(dir, "nonexistent.jsonl"))
		if got != "" {
			t.Errorf("extractPreview() = %q, want empty string", got)
		}
	})
}

func TestSearchInFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("finds matches", func(t *testing.T) {
		path := filepath.Join(dir, "search1.jsonl")
		lines := []map[string]interface{}{
			{"type": "human", "message": "Fix the database connection pool"},
			{"type": "assistant", "message": "I'll fix the database issue"},
			{"type": "human", "message": "Also update the tests"},
		}
		writeJSONL(t, path, lines)

		count := searchInFile(path, "database")
		if count != 2 {
			t.Errorf("searchInFile() = %d, want 2", count)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		path := filepath.Join(dir, "search2.jsonl")
		lines := []map[string]interface{}{
			{"type": "human", "message": "Fix the Database connection"},
			{"type": "assistant", "message": "DATABASE fixed"},
		}
		writeJSONL(t, path, lines)

		count := searchInFile(path, "database")
		if count != 2 {
			t.Errorf("searchInFile() = %d, want 2", count)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		path := filepath.Join(dir, "search3.jsonl")
		lines := []map[string]interface{}{
			{"type": "human", "message": "Hello world"},
		}
		writeJSONL(t, path, lines)

		count := searchInFile(path, "database")
		if count != 0 {
			t.Errorf("searchInFile() = %d, want 0", count)
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 5, "hello..."},
		{"with newlines", "hello\nworld", 20, "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestLoadProjects(t *testing.T) {
	// Create a temporary directory structure mimicking ~/.claude/projects/
	dir := t.TempDir()

	// Override the config to use our temp dir
	// We'll test with the direct loadSessions function instead
	projDir := filepath.Join(dir, "-Users-me-myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test session file
	sessionPath := filepath.Join(projDir, "abc-123-def.jsonl")
	lines := []map[string]interface{}{
		{"type": "human", "message": "Test message"},
	}
	writeJSONL(t, sessionPath, lines)

	sessions, err := loadSessions(projDir, "-Users-me-myproject", nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 1 {
		t.Fatalf("loadSessions() returned %d sessions, want 1", len(sessions))
	}

	s := sessions[0]
	if s.ID != "abc-123-def" {
		t.Errorf("session ID = %q, want %q", s.ID, "abc-123-def")
	}
	if s.ProjectPath != "/Users/me/myproject" {
		t.Errorf("session ProjectPath = %q, want %q", s.ProjectPath, "/Users/me/myproject")
	}
	if s.Preview != "Test message" {
		t.Errorf("session Preview = %q, want %q", s.Preview, "Test message")
	}
}

func TestTotalDiskUsage(t *testing.T) {
	projects := []*models.Project{
		{TotalSize: 1000},
		{TotalSize: 2000},
		{TotalSize: 3000},
	}

	got := TotalDiskUsage(projects)
	if got != 6000 {
		t.Errorf("TotalDiskUsage() = %d, want 6000", got)
	}
}

// writeJSONL is a test helper that writes JSON objects as lines.
func writeJSONL(t *testing.T, path string, objects []map[string]interface{}) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, obj := range objects {
		if err := enc.Encode(obj); err != nil {
			t.Fatal(err)
		}
	}
}
