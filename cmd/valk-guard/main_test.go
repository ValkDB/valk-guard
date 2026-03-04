// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

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

func TestRunScanParseErrorIsNonFatal(t *testing.T) {
	tmpDir := t.TempDir()
	sqlPath := filepath.Join(tmpDir, "broken.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT FROM;"), 0644); err != nil {
		t.Fatalf("failed to write SQL fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--format", "json"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d", exitSuccess, code)
	}
	if !strings.Contains(stderr.String(), "skipping unparseable SQL statement") {
		t.Fatalf("expected parse warning message, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "uses a PostgreSQL parser") {
		t.Fatalf("expected parse remediation hint, got %q", stderr.String())
	}
}

func TestRunScanNoScannableFilesFeedback(t *testing.T) {
	tmpDir := t.TempDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitSuccess, code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "0 findings") {
		t.Fatalf("expected 0 findings message, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "(no scannable files found)") {
		t.Fatalf("expected no scannable files message, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "no .sql, .go, or .py files found in scan paths") {
		t.Fatalf("expected warning about missing scannable files, got %q", stderr.String())
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

func TestRunScanWarnsOnUnknownRuleIDInConfig(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "ok.sql"), []byte("SELECT 1 LIMIT 1;"), 0644); err != nil {
		t.Fatalf("failed to write sql fixture: %v", err)
	}

	cfg := strings.Join([]string{
		"rules:",
		"  VG999:",
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
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", exitSuccess, code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown rule in config") {
		t.Fatalf("expected unknown-rule warning, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "VG999") {
		t.Fatalf("expected unknown rule id in warning, got %q", stderr.String())
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
		"  VG109: { enabled: false }",
		"  VG110: { enabled: false }",
		"  VG111: { enabled: false }",
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
		"  VG109: { enabled: false }",
		"  VG110: { enabled: false }",
		"  VG111: { enabled: false }",
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

func TestRunScanGoquQuerySchemaRules(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "db", "migrations"), 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	migration := strings.Join([]string{
		"CREATE TABLE users (id INTEGER, email TEXT);",
		"CREATE TABLE orders (id INTEGER, user_id INTEGER);",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "db", "migrations", "001.sql"), []byte(migration), 0644); err != nil {
		t.Fatalf("failed to write migration fixture: %v", err)
	}

	goSrc := strings.Join([]string{
		"package example",
		"",
		`import goqu "github.com/doug-martin/goqu/v9"`,
		"",
		"func build() {",
		`	_ = goqu.L("SELECT users.id, users.ghost FROM users INNER JOIN orders ON users.id = orders.bad_user_id WHERE orders.missing = 1")`,
		"}",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "query.go"), []byte(goSrc), 0644); err != nil {
		t.Fatalf("failed to write go fixture: %v", err)
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
		"  VG104: { enabled: false }",
		"  VG109: { enabled: false }",
		"  VG110: { enabled: false }",
		"  VG111: { enabled: false }",
		"  VG105:",
		"    enabled: true",
		"    engines: [goqu]",
		"  VG106:",
		"    enabled: true",
		"    engines: [goqu]",
		"",
	}, "\n")
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitFindings, code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG105"`) {
		t.Fatalf("expected VG105 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG106"`) {
		t.Fatalf("expected VG106 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "join predicate column") {
		t.Fatalf("expected INNER JOIN predicate validation finding, got %s", stdout.String())
	}
}

func TestRunScanSQLAlchemyQuerySchemaRules(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "db", "migrations"), 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	migration := strings.Join([]string{
		"CREATE TABLE users (id INTEGER, email TEXT);",
		"CREATE TABLE orders (id INTEGER, user_id INTEGER);",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "db", "migrations", "001.sql"), []byte(migration), 0644); err != nil {
		t.Fatalf("failed to write migration fixture: %v", err)
	}

	pySrc := strings.Join([]string{
		"from sqlalchemy import text",
		"",
		"def run(session):",
		`    session.execute(text("SELECT users.id, users.ghost FROM users INNER JOIN orders ON users.id = orders.bad_user_id WHERE orders.missing = 1"))`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "query.py"), []byte(pySrc), 0644); err != nil {
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
		"  VG104: { enabled: false }",
		"  VG109: { enabled: false }",
		"  VG110: { enabled: false }",
		"  VG111: { enabled: false }",
		"  VG105:",
		"    enabled: true",
		"    engines: [sqlalchemy]",
		"  VG106:",
		"    enabled: true",
		"    engines: [sqlalchemy]",
		"",
	}, "\n")
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitFindings, code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG105"`) {
		t.Fatalf("expected VG105 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG106"`) {
		t.Fatalf("expected VG106 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "join predicate column") {
		t.Fatalf("expected INNER JOIN predicate validation finding, got %s", stdout.String())
	}
}

func TestRunScanGoquQuerySchemaRulesUseGoModelColumns(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "db", "migrations"), 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	// Migration allows nickname, but Go model intentionally omits it.
	if err := os.WriteFile(
		filepath.Join(tmpDir, "db", "migrations", "001.sql"),
		[]byte("CREATE TABLE users (id INTEGER, email TEXT, nickname TEXT);"),
		0644,
	); err != nil {
		t.Fatalf("failed to write migration fixture: %v", err)
	}

	goSrc := strings.Join([]string{
		"package example",
		"",
		`import goqu "github.com/doug-martin/goqu/v9"`,
		"",
		"type User struct {",
		"    ID    int    `db:\"id\"`",
		"    Email string `db:\"email\"`",
		"}",
		"",
		"func (User) TableName() string { return \"users\" }",
		"",
		"func run() {",
		`    _ = goqu.L("SELECT users.nickname FROM users WHERE users.nickname = 'x'")`,
		"}",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "query.go"), []byte(goSrc), 0644); err != nil {
		t.Fatalf("failed to write go fixture: %v", err)
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
		"  VG104: { enabled: false }",
		"  VG109: { enabled: false }",
		"  VG110: { enabled: false }",
		"  VG111: { enabled: false }",
		"  VG105:",
		"    enabled: true",
		"    engines: [goqu]",
		"  VG106:",
		"    enabled: true",
		"    engines: [goqu]",
		"",
	}, "\n")
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitFindings, code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG105"`) {
		t.Fatalf("expected VG105 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG106"`) {
		t.Fatalf("expected VG106 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "nickname") {
		t.Fatalf("expected model-missing nickname finding, got %s", stdout.String())
	}
}

func TestRunScanSQLAlchemyQuerySchemaRulesUseSQLAlchemyModelColumns(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "db", "migrations"), 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	// Migration allows nickname, but SQLAlchemy model intentionally omits it.
	if err := os.WriteFile(
		filepath.Join(tmpDir, "db", "migrations", "001.sql"),
		[]byte("CREATE TABLE users (id INTEGER, email TEXT, nickname TEXT);"),
		0644,
	); err != nil {
		t.Fatalf("failed to write migration fixture: %v", err)
	}

	pySrc := strings.Join([]string{
		"from sqlalchemy import Column, Integer, String, text",
		"",
		"class User:",
		"    __tablename__ = \"users\"",
		"    id = Column(Integer, primary_key=True)",
		"    email = Column(String(255))",
		"",
		"def run(session):",
		"    session.execute(text(\"SELECT users.nickname FROM users WHERE users.nickname = 'x'\"))",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "query.py"), []byte(pySrc), 0644); err != nil {
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
		"  VG104: { enabled: false }",
		"  VG109: { enabled: false }",
		"  VG110: { enabled: false }",
		"  VG111: { enabled: false }",
		"  VG105:",
		"    enabled: true",
		"    engines: [sqlalchemy]",
		"  VG106:",
		"    enabled: true",
		"    engines: [sqlalchemy]",
		"",
	}, "\n")
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitFindings, code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG105"`) {
		t.Fatalf("expected VG105 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG106"`) {
		t.Fatalf("expected VG106 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "nickname") {
		t.Fatalf("expected model-missing nickname finding, got %s", stdout.String())
	}
}

func TestRunScanNewQuerySchemaRulesVG107VG108(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "db", "migrations"), 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(tmpDir, "db", "migrations", "001.sql"),
		[]byte(strings.Join([]string{
			"CREATE TABLE users (id INTEGER, email TEXT);",
			"CREATE TABLE orders (id INTEGER, user_id INTEGER);",
		}, "\n")),
		0644,
	); err != nil {
		t.Fatalf("failed to write migration fixture: %v", err)
	}

	querySQL := strings.Join([]string{
		"SELECT id FROM users u INNER JOIN orders o ON u.id = o.user_id;",
		"SELECT users.id FROM users INNER JOIN ghost_orders g ON users.id = g.user_id;",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "query.sql"), []byte(querySQL), 0644); err != nil {
		t.Fatalf("failed to write query fixture: %v", err)
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
		"  VG104: { enabled: false }",
		"  VG105: { enabled: false }",
		"  VG106: { enabled: false }",
		"  VG107:",
		"    enabled: true",
		"    engines: [sql]",
		"  VG108:",
		"    enabled: true",
		"    engines: [sql]",
		"",
	}, "\n")
	cfgPath := filepath.Join(tmpDir, ".valk-guard.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("failed to write config fixture: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--config", cfgPath, "--format", "json"}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitFindings, code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG107"`) {
		t.Fatalf("expected VG107 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG108"`) {
		t.Fatalf("expected VG108 finding in output, got %s", stdout.String())
	}
}

func TestRunScanNewSchemaRulesVG109VG110VG111(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "db", "migrations"), 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(tmpDir, "db", "migrations", "001.sql"),
		[]byte(strings.Join([]string{
			"CREATE TABLE users (id INTEGER, email TEXT);",
			"CREATE TABLE sessions (id INTEGER);",
			"CREATE TABLE orphan_table (id INTEGER);",
		}, "\n")),
		0644,
	); err != nil {
		t.Fatalf("failed to write migration fixture: %v", err)
	}

	goModels := strings.Join([]string{
		"package example",
		"",
		"type User struct {",
		"    ID       int    `db:\"id\"`",
		"    Email    string `db:\"email\"`",
		"    LegacyID int    `db:\"id\"`",
		"}",
		"",
		"type Session struct {",
		"    ID int",
		"}",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "models.go"), []byte(goModels), 0644); err != nil {
		t.Fatalf("failed to write go model fixture: %v", err)
	}

	cfg := strings.Join([]string{
		"go_model:",
		"  mapping_mode: balanced",
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
		"  VG104: { enabled: false }",
		"  VG105: { enabled: false }",
		"  VG106: { enabled: false }",
		"  VG107: { enabled: false }",
		"  VG108: { enabled: false }",
		"  VG109:",
		"    enabled: true",
		"    engines: [go]",
		"  VG110:",
		"    enabled: true",
		"    engines: [go]",
		"  VG111:",
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
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q, stdout=%q)", exitFindings, code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG109"`) {
		t.Fatalf("expected VG109 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG110"`) {
		t.Fatalf("expected VG110 finding in output, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"VG111"`) {
		t.Fatalf("expected VG111 finding in output, got %s", stdout.String())
	}
}

func TestRunScanOutputFileCreatedWhenOnlyParseWarnings(t *testing.T) {
	tmpDir := t.TempDir()
	sqlPath := filepath.Join(tmpDir, "broken.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT FROM;"), 0644); err != nil {
		t.Fatalf("failed to write SQL fixture: %v", err)
	}

	outputPath := filepath.Join(tmpDir, "results.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--format", "json", "--output", outputPath}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("expected exit code %d, got %d", exitSuccess, code)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
	if !strings.Contains(string(data), `"findings": []`) {
		t.Fatalf("expected empty findings JSON envelope, got: %s", string(data))
	}
}

func TestRunScanOutputFileCreatedOnSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	sqlPath := filepath.Join(tmpDir, "query.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT * FROM users;"), 0644); err != nil {
		t.Fatalf("failed to write SQL fixture: %v", err)
	}

	outputPath := filepath.Join(tmpDir, "results.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"scan", tmpDir, "--format", "json", "--output", outputPath}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", exitFindings, code, stderr.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
	if !strings.Contains(string(data), `"rule_id"`) {
		t.Fatalf("expected JSON findings in output file, got: %s", string(data))
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
