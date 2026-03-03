// Copyright 2025 ValkDB
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanSimpleSelect(t *testing.T) {
	s := &RawSQLScanner{}
	stmts, err := Collect(s.Scan(context.Background(), []string{filepath.Join("..", "..", "testdata", "sql", "simple_select.sql")}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}

	if stmts[0].SQL != "SELECT * FROM users" {
		t.Errorf("unexpected SQL: %q", stmts[0].SQL)
	}

	if stmts[0].Line != 1 {
		t.Errorf("expected line 1, got %d", stmts[0].Line)
	}
}

func TestScanMultiStatement(t *testing.T) {
	s := &RawSQLScanner{}
	stmts, err := Collect(s.Scan(context.Background(), []string{filepath.Join("..", "..", "testdata", "sql", "multi_statement.sql")}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(stmts))
	}

	// Verify line numbers.
	expectedLines := []int{1, 3, 5}
	for i, expected := range expectedLines {
		if stmts[i].Line != expected {
			t.Errorf("statement %d: expected line %d, got %d", i, expected, stmts[i].Line)
		}
	}
}

func TestScanWithIgnoreDirective(t *testing.T) {
	s := &RawSQLScanner{}
	stmts, err := Collect(s.Scan(context.Background(), []string{filepath.Join("..", "..", "testdata", "sql", "with_ignore.sql")}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}

	// First statement should have VG001 disabled.
	if len(stmts[0].Disabled) != 1 || stmts[0].Disabled[0] != "VG001" {
		t.Errorf("expected first statement to have VG001 disabled, got %v", stmts[0].Disabled)
	}

	// Second statement should have no disabled rules.
	if len(stmts[1].Disabled) != 0 {
		t.Errorf("expected second statement to have no disabled rules, got %v", stmts[1].Disabled)
	}
}

// TestScanNestedBlockComments verifies that nested /* /* */ */ are handled correctly.
func TestScanNestedBlockComments(t *testing.T) {
	s := &RawSQLScanner{}

	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "nested.sql")
	content := "/* outer /* inner */ end */ SELECT 1;"
	if err := os.WriteFile(sqlFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stmts, err := Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %+v", len(stmts), stmts)
	}

	// The comment should be preserved in the extracted text but not break the scanner.
	if !strings.Contains(stmts[0].SQL, "SELECT 1") {
		t.Errorf("expected statement to contain 'SELECT 1', got %q", stmts[0].SQL)
	}
}

// TestScanDollarQuoting verifies that dollar-quoted strings don't split on internal semicolons.
func TestScanDollarQuoting(t *testing.T) {
	s := &RawSQLScanner{}

	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "dollar.sql")
	content := `CREATE FUNCTION test() RETURNS void AS $$
BEGIN
    RAISE NOTICE 'hello;world';
END;
$$ LANGUAGE plpgsql;`
	if err := os.WriteFile(sqlFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stmts, err := Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	// The entire function body should be one statement.
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %+v", len(stmts), stmts)
	}

	if !strings.Contains(stmts[0].SQL, "LANGUAGE plpgsql") {
		t.Errorf("expected statement to contain 'LANGUAGE plpgsql', got %q", stmts[0].SQL)
	}
}

// TestScanTrailingStatementWithoutSemicolon verifies statements without trailing semicolons are captured.
func TestScanTrailingStatementWithoutSemicolon(t *testing.T) {
	s := &RawSQLScanner{}

	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "trailing.sql")
	if err := os.WriteFile(sqlFile, []byte("SELECT 1"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stmts, err := Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}

	if stmts[0].SQL != "SELECT 1" {
		t.Errorf("expected 'SELECT 1', got %q", stmts[0].SQL)
	}
}

// TestScanEmptyFile verifies that an empty SQL file produces no statements.
func TestScanEmptyFile(t *testing.T) {
	s := &RawSQLScanner{}

	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "empty.sql")
	if err := os.WriteFile(sqlFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stmts, err := Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 0 {
		t.Errorf("expected 0 statements, got %d", len(stmts))
	}
}

// TestScanSingleQuoteEscaping verifies that ” inside single-quoted strings is handled.
func TestScanSingleQuoteEscaping(t *testing.T) {
	s := &RawSQLScanner{}

	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "escape.sql")
	content := "SELECT 'it''s'; SELECT 2;"
	if err := os.WriteFile(sqlFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stmts, err := Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %+v", len(stmts), stmts)
	}

	if !strings.Contains(stmts[0].SQL, "it''s") {
		t.Errorf("expected escaped quote in first statement, got %q", stmts[0].SQL)
	}
}

// TestScanEStringEscaping verifies that PostgreSQL E-string backslash escapes
// are handled correctly: E'it\'s here' should be a single string literal.
func TestScanEStringEscaping(t *testing.T) {
	s := &RawSQLScanner{}

	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "estring.sql")
	content := `SELECT E'it\'s here'; SELECT 2;`
	if err := os.WriteFile(sqlFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stmts, err := Collect(s.Scan(context.Background(), []string{tmpDir}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %+v", len(stmts), stmts)
	}

	if !strings.Contains(stmts[0].SQL, `E'it\'s here'`) {
		t.Errorf("expected E-string with escaped quote in first statement, got %q", stmts[0].SQL)
	}

	if !strings.Contains(stmts[1].SQL, "SELECT 2") {
		t.Errorf("expected 'SELECT 2' in second statement, got %q", stmts[1].SQL)
	}
}

func TestScanDirectory(t *testing.T) {
	s := &RawSQLScanner{}
	stmts, err := Collect(s.Scan(context.Background(), []string{filepath.Join("..", "..", "testdata", "sql")}))
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) < 4 {
		t.Errorf("expected at least 4 statements from testdata/sql, got %d", len(stmts))
	}
}
