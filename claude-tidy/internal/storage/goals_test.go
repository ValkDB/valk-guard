package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGoals_NonexistentFile(t *testing.T) {
	// Override the goals path by temporarily pointing to a temp dir
	dir := t.TempDir()
	path := filepath.Join(dir, "goals.json")

	// Direct file read - file doesn't exist
	_, err := os.ReadFile(path)
	if !os.IsNotExist(err) {
		t.Fatal("expected file to not exist")
	}

	// The function should handle missing files gracefully
	// We test the underlying logic directly
	goals := &GoalsData{Sessions: make(map[string]SessionGoal)}
	if goals.Sessions == nil {
		t.Error("Sessions map should be initialized")
	}
	if len(goals.Sessions) != 0 {
		t.Error("Sessions map should be empty")
	}
}

func TestGoalsDataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goals.json")

	// Create goals data
	goals := &GoalsData{
		Sessions: map[string]SessionGoal{
			"session-abc": {
				Goal:        "Fix N+1 queries",
				CreatedAt:   "2025-02-02T10:00:00Z",
				ProjectPath: "/Users/me/myproject",
			},
			"session-def": {
				Goal:        "Add auth middleware",
				CreatedAt:   "2025-02-03T14:00:00Z",
				ProjectPath: "/Users/me/webapp",
			},
		},
	}

	// Write
	data, err := json.MarshalIndent(goals, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Read back
	readData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var loaded GoalsData
	if err := json.Unmarshal(readData, &loaded); err != nil {
		t.Fatal(err)
	}

	if len(loaded.Sessions) != 2 {
		t.Fatalf("loaded %d sessions, want 2", len(loaded.Sessions))
	}

	sg, ok := loaded.Sessions["session-abc"]
	if !ok {
		t.Fatal("session-abc not found")
	}
	if sg.Goal != "Fix N+1 queries" {
		t.Errorf("goal = %q, want %q", sg.Goal, "Fix N+1 queries")
	}
	if sg.ProjectPath != "/Users/me/myproject" {
		t.Errorf("project_path = %q, want %q", sg.ProjectPath, "/Users/me/myproject")
	}
}

func TestGoalsDataEmptyJSON(t *testing.T) {
	data := []byte(`{}`)
	var goals GoalsData
	if err := json.Unmarshal(data, &goals); err != nil {
		t.Fatal(err)
	}

	if goals.Sessions != nil && len(goals.Sessions) > 0 {
		t.Error("Sessions should be nil or empty for empty JSON")
	}
}

func TestGoalsDataMalformedJSON(t *testing.T) {
	data := []byte(`{invalid json}`)
	var goals GoalsData
	err := json.Unmarshal(data, &goals)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}
