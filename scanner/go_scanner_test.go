package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoScannerExtractsSQL(t *testing.T) {
	// Copy .go.txt fixture to a temp .go file for parsing.
	src, err := os.ReadFile(filepath.Join("..", "testdata", "go", "db_query.go.txt"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "db_query.go")
	if err := os.WriteFile(goFile, src, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &GoScanner{}
	stmts, err := s.Scan([]string{tmpDir})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 3 {
		t.Fatalf("expected 3 SQL statements, got %d", len(stmts))
	}

	expectedSQL := []string{
		"SELECT id, name FROM users WHERE active = true",
		"INSERT INTO logs (action) VALUES ('test')",
		"SELECT count(*) FROM orders",
	}

	for i, expected := range expectedSQL {
		if stmts[i].SQL != expected {
			t.Errorf("statement %d: expected %q, got %q", i, expected, stmts[i].SQL)
		}
	}
}

func TestGoScannerNoSQL(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "testdata", "go", "no_sql.go.txt"))
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "no_sql.go")
	if err := os.WriteFile(goFile, src, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &GoScanner{}
	stmts, err := s.Scan([]string{tmpDir})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 0 {
		t.Errorf("expected 0 SQL statements, got %d", len(stmts))
	}
}

func TestGoScannerSkipsNonSQLStringArgs(t *testing.T) {
	src := strings.Join([]string{
		"package example",
		"",
		"type DB interface {",
		"\tExec(query string, args ...any)",
		"}",
		"",
		"func run(db DB, query string) {",
		"\tdb.Exec(query, \"SELECT 1\")",
		"}",
	}, "\n")

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "false_positive.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &GoScanner{}
	stmts, err := s.Scan([]string{tmpDir})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 0 {
		t.Fatalf("expected 0 SQL statements, got %d: %+v", len(stmts), stmts)
	}
}

func TestGoScannerContextMethodsUseSecondArg(t *testing.T) {
	src := strings.Join([]string{
		"package example",
		"",
		"import \"context\"",
		"",
		"type DB interface {",
		"\tQueryContext(ctx context.Context, query string, args ...any)",
		"\tExecContext(ctx context.Context, query string, args ...any)",
		"\tQueryRowContext(ctx context.Context, query string, args ...any)",
		"}",
		"",
		"func run(ctx context.Context, db DB) {",
		"\tdb.QueryContext(ctx, \"SELECT 1\")",
		"\tdb.ExecContext(ctx, \"UPDATE users SET active = true\")",
		"\tdb.QueryRowContext(ctx, \"SELECT count(*) FROM users\")",
		"}",
	}, "\n")

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "context_methods.go")
	if err := os.WriteFile(goFile, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s := &GoScanner{}
	stmts, err := s.Scan([]string{tmpDir})
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(stmts) != 3 {
		t.Fatalf("expected 3 SQL statements, got %d", len(stmts))
	}

	expectedSQL := []string{
		"SELECT 1",
		"UPDATE users SET active = true",
		"SELECT count(*) FROM users",
	}

	for i, expected := range expectedSQL {
		if stmts[i].SQL != expected {
			t.Errorf("statement %d: expected %q, got %q", i, expected, stmts[i].SQL)
		}
	}
}
