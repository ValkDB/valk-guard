package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunScanJSONFindingsExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	sqlPath := filepath.Join(tmpDir, "query.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT * FROM users;"), 0644); err != nil {
		t.Fatalf("failed to write SQL fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--format", "json"}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", exitFindings, code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"rule_id"`) {
		t.Fatalf("expected JSON findings output, got: %s", stdout.String())
	}
}

func TestRunScanParseErrorExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	sqlPath := filepath.Join(tmpDir, "broken.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT FROM;"), 0644); err != nil {
		t.Fatalf("failed to write SQL fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--format", "json"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("expected exit code %d, got %d", exitError, code)
	}
	if !strings.Contains(stderr.String(), "parse error at") {
		t.Fatalf("expected parse error message, got %q", stderr.String())
	}
}

func TestRunScanExcludedBrokenGoFileDoesNotFail(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte("exclude:\n  - broken.go\n"), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "broken.go"), []byte("this is not valid go"), 0644); err != nil {
		t.Fatalf("failed to write go fixture: %v", err)
	}
	// Clean SQL that should not trigger rules.
	if err := os.WriteFile(filepath.Join(tmpDir, "ok.sql"), []byte("SELECT 1 LIMIT 1;"), 0644); err != nil {
		t.Fatalf("failed to write sql fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", exitSuccess, code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRunScanEmptyConfigFormatFallsBackToTerminal(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte("format: \"\"\n"), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ok.sql"), []byte("SELECT 1 LIMIT 1;"), 0644); err != nil {
		t.Fatalf("failed to write sql fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", exitSuccess, code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "0 findings") {
		t.Fatalf("expected terminal output fallback, got %q", stdout.String())
	}
}

func TestRunScanInvalidLogLevelExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "ok.sql"), []byte("SELECT 1 LIMIT 1;"), 0644); err != nil {
		t.Fatalf("failed to write sql fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--log-level", "trace"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("expected exit code %d, got %d", exitError, code)
	}
	if !strings.Contains(stderr.String(), "invalid log level") {
		t.Fatalf("expected invalid log level message, got %q", stderr.String())
	}
}

func TestRunScanRuleEngineFilter(t *testing.T) {
	tmpDir := t.TempDir()

	goSrc := strings.Join([]string{
		"package example",
		"",
		"type DB interface { Query(string) }",
		"",
		"func run(db DB) {",
		`	db.Query("SELECT * FROM users")`,
		"}",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "query.go"), []byte(goSrc), 0644); err != nil {
		t.Fatalf("failed to write go fixture: %v", err)
	}

	cfg := strings.Join([]string{
		"rules:",
		"  VG001:",
		"    engines: [sql]",
		"  VG004:",
		"    enabled: false",
		"",
	}, "\n")
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitSuccess, code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), `"rule_id"`) {
		t.Fatalf("expected no findings due to engine filter, got %s", stdout.String())
	}
}

func TestRunScanSchemaRuleEngineFilterRespected(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "db", "migrations"), 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "db", "migrations", "001.sql"), []byte("CREATE TABLE users (id INTEGER);"), 0644); err != nil {
		t.Fatalf("failed to write migration fixture: %v", err)
	}
	py := strings.Join([]string{
		"from sqlalchemy import Column, Integer",
		"",
		"class Account:",
		"    __tablename__ = \"accounts\"",
		"    id = Column(Integer)",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "models.py"), []byte(py), 0644); err != nil {
		t.Fatalf("failed to write python fixture: %v", err)
	}

	cfg := strings.Join([]string{
		"rules:",
		"  VG001: { enabled: false }",
		"  VG002: { enabled: false }",
		"  VG003: { enabled: false }",
		"  VG004: { enabled: false }",
		"  VG005: { enabled: false }",
		"  VG006: { enabled: false }",
		"  VG007: { enabled: false }",
		"  VG008: { enabled: false }",
		"  VG101: { enabled: false }",
		"  VG102: { enabled: false }",
		"  VG103: { enabled: false }",
		"  VG104:",
		"    enabled: true",
		"    engines: [go]",
		"",
	}, "\n")
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitSuccess, code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), `"rule_id"`) {
		t.Fatalf("expected no findings due to schema engine filter, got %s", stdout.String())
	}
}

func TestRunScanSchemaPrefersMigrationPaths(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "db", "migrations"), 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "testdata"), 0755); err != nil {
		t.Fatalf("failed to create testdata dir: %v", err)
	}
	// Canonical migration schema.
	if err := os.WriteFile(filepath.Join(tmpDir, "db", "migrations", "001.sql"), []byte("CREATE TABLE users (email TEXT);"), 0644); err != nil {
		t.Fatalf("failed to write migration fixture: %v", err)
	}
	// Conflicting non-migration SQL that should be ignored when migrations exist.
	if err := os.WriteFile(filepath.Join(tmpDir, "testdata", "fixture.sql"), []byte("CREATE TABLE users (email INTEGER);"), 0644); err != nil {
		t.Fatalf("failed to write fixture sql: %v", err)
	}
	py := strings.Join([]string{
		"from sqlalchemy import Column, String",
		"",
		"class User:",
		"    __tablename__ = \"users\"",
		"    email = Column(String(255))",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "models.py"), []byte(py), 0644); err != nil {
		t.Fatalf("failed to write python fixture: %v", err)
	}

	cfg := strings.Join([]string{
		"rules:",
		"  VG001: { enabled: false }",
		"  VG002: { enabled: false }",
		"  VG003: { enabled: false }",
		"  VG004: { enabled: false }",
		"  VG005: { enabled: false }",
		"  VG006: { enabled: false }",
		"  VG007: { enabled: false }",
		"  VG008: { enabled: false }",
		"  VG101: { enabled: false }",
		"  VG102: { enabled: false }",
		"  VG104: { enabled: false }",
		"",
	}, "\n")
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitSuccess, code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), `"VG103"`) {
		t.Fatalf("expected no VG103 mismatch when migration paths are preferred, got %s", stdout.String())
	}
}

func TestIsMigrationSQLFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/repo/db/migrations/001.sql", want: true},
		{path: "C:\\repo\\db\\migration\\002.sql", want: true},
		{path: "/repo/migrate/003.sql", want: true},
		{path: "/repo/testdata/fixture.sql", want: false},
		{path: "/repo/db/migrations/readme.txt", want: false},
	}

	for _, tt := range tests {
		if got := isMigrationSQLFile(tt.path); got != tt.want {
			t.Errorf("isMigrationSQLFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
