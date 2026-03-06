// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valkdb/valk-guard/internal/rules"
	"github.com/valkdb/valk-guard/internal/scanner"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Format != "terminal" {
		t.Errorf("expected default format 'terminal', got %q", cfg.Format)
	}
	if cfg.Rules == nil {
		t.Error("expected non-nil Rules map")
	}
	if cfg.GoModel.MappingMode != GoModelMappingStrict {
		t.Errorf("expected default go_model.mapping_mode %q, got %q", GoModelMappingStrict, cfg.GoModel.MappingMode)
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

	if len(cfg.MigrationPaths) != 2 {
		t.Errorf("expected 2 migration paths, got %d", len(cfg.MigrationPaths))
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
		{
			name:     "multiple doublestar segments match nested path",
			exclude:  []string{"vendor/**/migrations/**/*.sql"},
			filePath: "vendor/v2/migrations/2024/001_init.sql",
			want:     true,
		},
		{
			name:     "multiple doublestar segments no match wrong prefix",
			exclude:  []string{"vendor/**/migrations/**/*.sql"},
			filePath: "src/v2/migrations/2024/001_init.sql",
			want:     false,
		},
		{
			name:     "multiple doublestar segments no match wrong middle",
			exclude:  []string{"vendor/**/migrations/**/*.sql"},
			filePath: "vendor/v2/seeds/2024/001_init.sql",
			want:     false,
		},
		{
			name:     "multiple doublestar segments no match wrong extension",
			exclude:  []string{"vendor/**/migrations/**/*.sql"},
			filePath: "vendor/v2/migrations/2024/001_init.txt",
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

func TestIsMigrationPath(t *testing.T) {
	tests := []struct {
		name           string
		migrationPaths []string
		filePath       string
		want           bool
	}{
		{
			name:     "default matches nested migrations dir",
			filePath: "/repo/db/migrations/001.sql",
			want:     true,
		},
		{
			name:     "default matches relative migrations dir",
			filePath: "db/migrations/001.sql",
			want:     true,
		},
		{
			name:     "default matches windows migration dir",
			filePath: `C:\repo\db\migration\002.sql`,
			want:     true,
		},
		{
			name:     "default does not match non sql",
			filePath: "/repo/db/migrate/readme.txt",
			want:     false,
		},
		{
			name:           "custom directory matches all sql beneath it",
			migrationPaths: []string{"db/changes"},
			filePath:       "/repo/db/changes/001_init.sql",
			want:           true,
		},
		{
			name:           "custom glob matches schema files",
			migrationPaths: []string{"schema/**/*.sql"},
			filePath:       "/repo/schema/ddl/001_init.sql",
			want:           true,
		},
		{
			name:           "custom paths disable defaults",
			migrationPaths: []string{"db/changes"},
			filePath:       "/repo/db/migrations/001_init.sql",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MigrationPaths: tt.migrationPaths}
			if err := validateConfig(cfg); err != nil {
				t.Fatalf("validateConfig() error = %v", err)
			}
			got := cfg.IsMigrationPath(tt.filePath)
			if got != tt.want {
				t.Errorf("IsMigrationPath(%q) = %v, want %v (patterns: %v)", tt.filePath, got, tt.want, tt.migrationPaths)
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

func TestLoadValidRDJSONLFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rdjsonl.yaml")
	data := []byte("format: rdjsonl\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected rdjsonl format to load successfully, got: %v", err)
	}
	if cfg.Format != FormatRDJSONL {
		t.Fatalf("expected format %q, got %q", FormatRDJSONL, cfg.Format)
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

func TestLoadInvalidRuleEngine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-engine.yaml")
	data := []byte("rules:\n  VG001:\n    engines: [oracle]\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid engine, got nil")
	}
}

func TestLoadInvalidGoModelMappingMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-go-model-mode.yaml")
	data := []byte("go_model:\n  mapping_mode: custom\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid go_model.mapping_mode, got nil")
	}
}

func TestLoadNormalizesGoModelMappingMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go-model-mode.yaml")
	data := []byte("go_model:\n  mapping_mode: PERMISSIVE\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.GoModel.MappingMode != GoModelMappingPermissive {
		t.Fatalf("expected normalized mode %q, got %q", GoModelMappingPermissive, cfg.GoModel.MappingMode)
	}
}

func TestIsRuleEnabledForEngine(t *testing.T) {
	cfg := Default()
	cfg.Rules["VG001"] = RuleConfig{Engines: []string{"goqu", "sqlalchemy"}}
	cfg.Rules["VG002"] = RuleConfig{Engines: []string{"all"}}

	if !cfg.IsRuleEnabledForEngine("VG001", scanner.EngineGoqu) {
		t.Fatal("expected VG001 to be enabled for goqu")
	}
	if !cfg.IsRuleEnabledForEngine("VG001", scanner.EngineSQLAlchemy) {
		t.Fatal("expected VG001 to be enabled for sqlalchemy")
	}
	if cfg.IsRuleEnabledForEngine("VG001", scanner.EngineSQL) {
		t.Fatal("expected VG001 to be disabled for raw sql")
	}
	if !cfg.IsRuleEnabledForEngine("VG002", scanner.EngineGo) {
		t.Fatal("expected VG002 with engines=[all] to be enabled")
	}
	if !cfg.IsRuleEnabledForEngine("VG999", scanner.EngineSQL) {
		t.Fatal("expected unknown rule to be enabled by default")
	}
}
