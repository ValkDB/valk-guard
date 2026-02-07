package scanner

import (
	"path/filepath"
	"testing"
)

func TestScanSimpleSelect(t *testing.T) {
	s := &RawSQLScanner{}
	stmts, err := s.Scan([]string{filepath.Join("..", "testdata", "sql", "simple_select.sql")})
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
	stmts, err := s.Scan([]string{filepath.Join("..", "testdata", "sql", "multi_statement.sql")})
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
	stmts, err := s.Scan([]string{filepath.Join("..", "testdata", "sql", "with_ignore.sql")})
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

func TestScanDirectory(t *testing.T) {
	s := &RawSQLScanner{}
	stmts, err := s.Scan([]string{filepath.Join("..", "testdata", "sql")})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) < 4 {
		t.Errorf("expected at least 4 statements from testdata/sql, got %d", len(stmts))
	}
}
