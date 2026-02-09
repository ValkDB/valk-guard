package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Format != "terminal" {
		t.Errorf("expected default format 'terminal', got %q", cfg.Format)
	}
	if cfg.Rules == nil {
		t.Error("expected non-nil Rules map")
	}
}

func TestLoadFull(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "testdata", "config", "full.yaml"))
	if err != nil {
		t.Fatalf("failed to load full config: %v", err)
	}

	if cfg.Format != "json" {
		t.Errorf("expected format 'json', got %q", cfg.Format)
	}

	if len(cfg.Exclude) != 2 {
		t.Errorf("expected 2 excludes, got %d", len(cfg.Exclude))
	}

	if len(cfg.Rules) != 3 {
		t.Errorf("expected 3 rules, got %d", len(cfg.Rules))
	}
}

func TestLoadMinimal(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "testdata", "config", "minimal.yaml"))
	if err != nil {
		t.Fatalf("failed to load minimal config: %v", err)
	}

	if cfg.Format != "terminal" {
		t.Errorf("expected format 'terminal', got %q", cfg.Format)
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected no error for missing config, got: %v", err)
	}
	if cfg.Format != "terminal" {
		t.Errorf("expected default format, got %q", cfg.Format)
	}
}

func TestIsRuleEnabled(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "testdata", "config", "full.yaml"))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	tests := []struct {
		name   string
		ruleID string
		want   bool
	}{
		{
			name:   "explicitly enabled rule",
			ruleID: "VG001",
			want:   true,
		},
		{
			name:   "explicitly disabled rule",
			ruleID: "VG002",
			want:   false,
		},
		{
			name:   "unknown rule enabled by default",
			ruleID: "VG999",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.IsRuleEnabled(tt.ruleID)
			if got != tt.want {
				t.Errorf("IsRuleEnabled(%q) = %v, want %v", tt.ruleID, got, tt.want)
			}
		})
	}
}

func TestRuleSeverity(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "testdata", "config", "full.yaml"))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	tests := []struct {
		name       string
		ruleID     string
		defaultSev rules.Severity
		want       rules.Severity
	}{
		{
			name:       "overridden severity",
			ruleID:     "VG001",
			defaultSev: rules.SeverityWarning,
			want:       rules.SeverityError,
		},
		{
			name:       "unknown rule uses default severity",
			ruleID:     "VG999",
			defaultSev: rules.SeverityWarning,
			want:       rules.SeverityWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.RuleSeverity(tt.ruleID, tt.defaultSev)
			if got != tt.want {
				t.Errorf("RuleSeverity(%q, %s) = %s, want %s", tt.ruleID, tt.defaultSev, got, tt.want)
			}
		})
	}
}

func TestShouldExclude(t *testing.T) {
	tests := []struct {
		name     string
		exclude  []string
		filePath string
		want     bool
	}{
		{
			name:     "matching gen.sql pattern",
			exclude:  []string{"vendor/*", "*.gen.sql"},
			filePath: "schema.gen.sql",
			want:     true,
		},
		{
			name:     "non-matching plain sql file",
			exclude:  []string{"vendor/*", "*.gen.sql"},
			filePath: "schema.sql",
			want:     false,
		},
		{
			name:     "doublestar matches nested path",
			exclude:  []string{"vendor/**"},
			filePath: "vendor/lib/foo.sql",
			want:     true,
		},
		{
			name:     "doublestar matches windows-style path separators",
			exclude:  []string{"vendor/**"},
			filePath: `vendor\lib\foo.sql`,
			want:     true,
		},
		{
			name:     "doublestar matches absolute path",
			exclude:  []string{"vendor/**"},
			filePath: "/abs/project/vendor/lib/foo.sql",
			want:     true,
		},
		{
			name:     "doublestar matches direct child",
			exclude:  []string{"vendor/**"},
			filePath: "vendor/foo.sql",
			want:     true,
		},
		{
			name:     "doublestar does not match unrelated path",
			exclude:  []string{"vendor/**"},
			filePath: "src/main.sql",
			want:     false,
		},
		{
			name:     "doublestar suffix matches nested migration",
			exclude:  []string{"**/migrations/*.sql"},
			filePath: "db/migrations/001_init.sql",
			want:     true,
		},
		{
			name:     "doublestar suffix matches root migration",
			exclude:  []string{"**/migrations/*.sql"},
			filePath: "migrations/002_add_users.sql",
			want:     true,
		},
		{
			name:     "doublestar suffix matches deeply nested migration",
			exclude:  []string{"**/migrations/*.sql"},
			filePath: "src/db/migrations/003_create_table.sql",
			want:     true,
		},
		{
			name:     "doublestar suffix does not match non-migration dir",
			exclude:  []string{"**/migrations/*.sql"},
			filePath: "db/seeds/001_init.sql",
			want:     false,
		},
		{
			name:     "doublestar suffix does not match subdir under migrations",
			exclude:  []string{"**/migrations/*.sql"},
			filePath: "migrations/subdir/001_init.sql",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Exclude: tt.exclude}
			got := cfg.ShouldExclude(tt.filePath)
			if got != tt.want {
				t.Errorf("ShouldExclude(%q) = %v, want %v (patterns: %v)", tt.filePath, got, tt.want, tt.exclude)
			}
		})
	}
}

func TestLoadInvalidFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-format.yaml")
	data := []byte("format: xml\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
}

func TestLoadInvalidSeverity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	data := []byte("rules:\n  VG001:\n    severity: critical\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid severity, got nil")
	}
}
