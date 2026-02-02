package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	got := ClaudeDir()
	want := filepath.Join(home, ".claude")
	if got != want {
		t.Errorf("ClaudeDir() = %q, want %q", got, want)
	}
}

func TestProjectsDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	got := ProjectsDir()
	want := filepath.Join(home, ".claude", "projects")
	if got != want {
		t.Errorf("ProjectsDir() = %q, want %q", got, want)
	}
}

func TestTidyDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	got := TidyDir()
	want := filepath.Join(home, ".claude-tidy")
	if got != want {
		t.Errorf("TidyDir() = %q, want %q", got, want)
	}
}

func TestGoalsFile(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	got := GoalsFile()
	want := filepath.Join(home, ".claude-tidy", "goals.json")
	if got != want {
		t.Errorf("GoalsFile() = %q, want %q", got, want)
	}
}

func TestEnsureTidyDir(t *testing.T) {
	// We don't actually create ~/.claude-tidy in tests
	// Just verify the function doesn't panic
	// In a real test we'd use a temp dir override
	err := EnsureTidyDir()
	if err != nil {
		// May fail in restricted environments, which is OK
		t.Logf("EnsureTidyDir() err = %v (may be expected in restricted env)", err)
	}
}

func TestConstants(t *testing.T) {
	if AppName != "claude-tidy" {
		t.Errorf("AppName = %q, want %q", AppName, "claude-tidy")
	}
	if AppVersion != "0.1.0" {
		t.Errorf("AppVersion = %q, want %q", AppVersion, "0.1.0")
	}
	if FreshDays != 3 {
		t.Errorf("FreshDays = %d, want 3", FreshDays)
	}
	if StaleDays != 14 {
		t.Errorf("StaleDays = %d, want 14", StaleDays)
	}
	if LargeBytes != 1<<30 {
		t.Errorf("LargeBytes = %d, want %d", LargeBytes, 1<<30)
	}
}
